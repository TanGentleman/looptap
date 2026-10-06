// Package claudeenv reconstructs the environment Claude Code hands to Bash.
//
// The model never sees process.env directly — it sees whatever a Bash tool
// call would print. That is this process's environment, overlaid with the
// env maps Claude reads from ~/.claude.json and the settings stack, then
// (when CLAUDE_CODE_SUBPROCESS_ENV_SCRUB is on) with the same credential
// names stripped that Claude strips before it spawns a shell.
package claudeenv

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Input is everything Resolve needs, and everything a test wants to fake.
// Zero values mean "ask the operating system".
type Input struct {
	// Dir is the directory Claude was launched in. Empty means the current
	// working directory.
	Dir string
	// Environ is the shell environment Claude inherits, in KEY=value form.
	// Nil means os.Environ(); an empty slice means a genuinely empty environment.
	Environ []string
	// Home overrides the user home directory. Empty means os.UserHomeDir.
	Home string
	// ManagedDir overrides the managed-settings directory
	// (/etc/claude-code, or the platform equivalent). Empty means the default.
	ManagedDir string
	// DiscoverGit finds the main checkout that owns Dir, for
	// .claude/settings.local.json. Nil means ask git. Return "" when Dir
	// is not in a repository.
	DiscoverGit func(dir string) (string, error)
}

// Result is the environment a Claude Code Bash command would see.
type Result struct {
	// Lines are KEY=value, sorted by name.
	Lines []string
	// Warnings are non-fatal problems (a settings file that isn't JSON, a
	// home directory we couldn't find). The command still prints Lines.
	Warnings []string
}

// Resolve builds the environment Claude's Bash tool can read.
func Resolve(in Input) (Result, error) {
	dir := in.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return Result{}, fmt.Errorf("finding working directory: %w", err)
		}
	}
	dir = filepath.Clean(dir)

	environ := in.Environ
	if environ == nil {
		environ = os.Environ()
	}
	env := parseEnviron(environ)

	home := in.Home
	var warnings []string
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("no home directory: %v", err))
		} else {
			home = h
		}
	}

	managedDir := in.ManagedDir
	if managedDir == "" {
		managedDir = defaultManagedDir()
	}

	configHome, globalPath := configPaths(env, home)

	// ~/.claude.json (or the legacy .config.json) lands first, then the
	// merged settings env overwrites it. Same order as
	// applyConfigEnvironmentVariables.
	global, warn := loadEnvFile(globalPath)
	warnings = appendWarning(warnings, warn)
	applyFiltered(env, global)

	merged := map[string]string{}
	if configHome != "" {
		user, warn := loadEnvFile(filepath.Join(configHome, "settings.json"))
		warnings = appendWarning(warnings, warn)
		mergeInto(merged, user)
	}

	project, warn := loadEnvFile(filepath.Join(dir, ".claude", "settings.json"))
	warnings = appendWarning(warnings, warn)
	mergeInto(merged, project)

	localCwd, warn := loadEnvFile(filepath.Join(dir, ".claude", "settings.local.json"))
	warnings = appendWarning(warnings, warn)
	mergeInto(merged, localCwd)

	gitRoot, err := gitRootFor(in, dir)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("git root: %v", err))
	} else if gitRoot != "" && filepath.Clean(gitRoot) != dir {
		localRoot, warn := loadEnvFile(filepath.Join(gitRoot, ".claude", "settings.local.json"))
		warnings = appendWarning(warnings, warn)
		mergeInto(merged, localRoot)
	}

	managed, warns := loadManaged(managedDir)
	warnings = append(warnings, warns...)
	mergeInto(merged, managed)

	applyFiltered(env, merged)
	scrubSubprocess(env)

	return Result{Lines: formatEnv(env), Warnings: warnings}, nil
}

func gitRootFor(in Input, dir string) (string, error) {
	discover := in.DiscoverGit
	if discover == nil {
		discover = discoverGitRoot
	}
	return discover(dir)
}

func appendWarning(warnings []string, warn string) []string {
	if warn == "" {
		return warnings
	}
	return append(warnings, warn)
}

func parseEnviron(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, kv := range environ {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		env[kv[:i]] = kv[i+1:]
	}
	return env
}

func formatEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + env[k]
	}
	return lines
}

// configPaths mirrors getClaudeConfigHomeDir and getGlobalClaudeFile.
// CLAUDE_CONFIG_DIR relocates both; otherwise settings live in ~/.claude
// and the global file is ~/.claude.json.
func configPaths(env map[string]string, home string) (configHome, globalPath string) {
	configDir := env["CLAUDE_CONFIG_DIR"]
	switch {
	case configDir != "":
		configHome = configDir
	case home != "":
		configHome = filepath.Join(home, ".claude")
	}

	if configHome != "" {
		legacy := filepath.Join(configHome, ".config.json")
		if st, err := os.Stat(legacy); err == nil && st.Mode().IsRegular() {
			return configHome, legacy
		}
	}

	base := configDir
	if base == "" {
		base = home
	}
	if base == "" {
		return configHome, ""
	}
	return configHome, filepath.Join(base, ".claude.json")
}

func defaultManagedDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "windows":
		return `C:\Program Files\ClaudeCode`
	default:
		return "/etc/claude-code"
	}
}

// applyFiltered copies src onto dst, dropping keys Claude refuses to let
// settings clobber: the ssh-tunnel auth vars when ANTHROPIC_UNIX_SOCKET is
// already set, and provider-routing vars when the host owns inference.
func applyFiltered(dst, src map[string]string) {
	if len(src) == 0 {
		return
	}
	drop := dropSettingsKey(dst)
	for k, v := range src {
		if drop(k) {
			continue
		}
		dst[k] = v
	}
}

func dropSettingsKey(base map[string]string) func(string) bool {
	tunnel := base["ANTHROPIC_UNIX_SOCKET"] != ""
	hostManaged := isEnvTruthy(base["CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST"])
	return func(k string) bool {
		if tunnel && tunnelVars[k] {
			return true
		}
		if hostManaged && providerManaged(k) {
			return true
		}
		return false
	}
}

func mergeInto(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func scrubSubprocess(env map[string]string) {
	if !isEnvTruthy(env["CLAUDE_CODE_SUBPROCESS_ENV_SCRUB"]) {
		return
	}
	for _, k := range subprocessScrub {
		delete(env, k)
		delete(env, "INPUT_"+k)
	}
}

func isEnvTruthy(v string) bool {
	if v == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func providerManaged(key string) bool {
	upper := strings.ToUpper(key)
	if providerManagedVars[upper] {
		return true
	}
	return strings.HasPrefix(upper, "VERTEX_REGION_CLAUDE_")
}

// loadEnvFile reads the "env" object from a Claude settings or ~/.claude.json
// file. A missing file is not an error — most of these are optional.
func loadEnvFile(path string) (map[string]string, string) {
	if path == "" {
		return nil, ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, fmt.Sprintf("skipping %s: %v", path, err)
	}
	env, err := parseEnvObject(data)
	if err != nil {
		// Invalid JSON drops the file. A non-string key keeps the other
		// keys and still reports the one it skipped.
		if env == nil {
			return nil, fmt.Sprintf("skipping %s: %v", path, err)
		}
		return env, fmt.Sprintf("%s: %v", path, err)
	}
	return env, ""
}

func loadManaged(dir string) (map[string]string, []string) {
	merged := map[string]string{}
	var warnings []string

	base, warn := loadEnvFile(filepath.Join(dir, "managed-settings.json"))
	warnings = appendWarning(warnings, warn)
	mergeInto(merged, base)

	entries, err := os.ReadDir(filepath.Join(dir, "managed-settings.d"))
	if err != nil {
		if !os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("skipping %s: %v", filepath.Join(dir, "managed-settings.d"), err))
		}
		return merged, warnings
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if e.IsDir() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		overlay, warn := loadEnvFile(filepath.Join(dir, "managed-settings.d", name))
		warnings = appendWarning(warnings, warn)
		mergeInto(merged, overlay)
	}
	return merged, warnings
}

// parseEnvObject pulls the env map out of a settings document. Values that
// aren't strings are coerced when that's unambiguous (numbers, bools) and
// skipped with an error when they aren't — one bad key does not sink the
// rest of the file, but a document that isn't JSON does.
func parseEnvObject(data []byte) (map[string]string, error) {
	var doc struct {
		Env json.RawMessage `json:"env"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON")
	}
	if len(bytes.TrimSpace(doc.Env)) == 0 || bytes.Equal(bytes.TrimSpace(doc.Env), []byte("null")) {
		return nil, nil
	}

	dec := json.NewDecoder(bytes.NewReader(doc.Env))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("env is not an object")
	}

	out := make(map[string]string, len(raw))
	var bad []string
	for k, v := range raw {
		if v == nil {
			continue
		}
		s, ok := coerceEnvValue(v)
		if !ok {
			bad = append(bad, k)
			continue
		}
		out[k] = s
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return out, fmt.Errorf("env.%s is not a string", strings.Join(bad, ", env."))
	}
	return out, nil
}

func coerceEnvValue(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	case bool:
		return strconv.FormatBool(t), true
	default:
		return "", false
	}
}

// discoverGitRoot returns the main checkout containing dir, which is where
// Claude reads .claude/settings.local.json. Not a repository is ("", nil).
func discoverGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}
	common := strings.TrimSpace(string(out))
	if common == "" {
		return "", nil
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	if filepath.Base(common) != ".git" {
		return "", nil
	}
	return filepath.Dir(common), nil
}
