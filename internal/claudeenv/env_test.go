package claudeenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestResolve_SettingsStack(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	proj := filepath.Join(root, "proj")
	managed := filepath.Join(root, "managed")
	gitRoot := filepath.Join(root, "git")

	writeFile(t, filepath.Join(home, ".claude.json"), `{
		"oauthAccount": {"accessToken": "super-secret-token"},
		"env": {"FROM": "global", "ONLY_GLOBAL": "1"}
	}`)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{
		"env": {"FROM": "user", "ONLY_USER": "1"}
	}`)
	writeFile(t, filepath.Join(proj, ".claude", "settings.json"), `{
		"env": {"FROM": "project", "ONLY_PROJECT": "1"}
	}`)
	writeFile(t, filepath.Join(proj, ".claude", "settings.local.json"), `{
		"env": {"FROM": "local-cwd", "ONLY_LOCAL": "cwd"}
	}`)
	writeFile(t, filepath.Join(gitRoot, ".claude", "settings.local.json"), `{
		"env": {"FROM": "local-root"}
	}`)
	writeFile(t, filepath.Join(managed, "managed-settings.json"), `{
		"env": {"FROM": "managed", "ONLY_MANAGED": "base"}
	}`)
	writeFile(t, filepath.Join(managed, "managed-settings.d", "20-b.json"), `{
		"env": {"FROM": "drop-b"}
	}`)
	writeFile(t, filepath.Join(managed, "managed-settings.d", "10-a.json"), `{
		"env": {"FROM": "drop-a", "DROP": "a"}
	}`)
	writeFile(t, filepath.Join(managed, "managed-settings.d", ".hidden.json"), `{
		"env": {"HIDDEN": "nope"}
	}`)
	writeFile(t, filepath.Join(managed, "managed-settings.d", "notes.txt"), `{"env":{"IGNORED":"1"}}`)

	res := mustResolve(t, Input{
		Dir:        proj,
		Home:       home,
		ManagedDir: managed,
		Environ:    []string{"FROM=shell", "ONLY_SHELL=1", "PATH=/usr/bin"},
		DiscoverGit: func(string) (string, error) {
			return gitRoot, nil
		},
	})
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}

	got := envMap(t, res.Lines)
	want := map[string]string{
		"FROM":         "drop-b",
		"ONLY_GLOBAL":  "1",
		"ONLY_USER":    "1",
		"ONLY_PROJECT": "1",
		"ONLY_LOCAL":   "cwd",
		"ONLY_MANAGED": "base",
		"DROP":         "a",
		"ONLY_SHELL":   "1",
		"PATH":         "/usr/bin",
	}
	assertEnv(t, got, want)
	if _, ok := got["HIDDEN"]; ok {
		t.Fatalf("dotfile drop-in leaked: %v", got)
	}
	if _, ok := got["IGNORED"]; ok {
		t.Fatalf("non-json drop-in leaked: %v", got)
	}
	joined := strings.Join(res.Lines, "\n")
	if strings.Contains(joined, "super-secret-token") {
		t.Fatalf("global config leaked a non-env field:\n%s", joined)
	}
	if !sort.StringsAreSorted(res.Lines) {
		t.Fatalf("lines not sorted:\n%s", joined)
	}
}

func TestResolve_LegacyConfigAndConfigDir(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "cfg")
	writeFile(t, filepath.Join(configDir, ".config.json"), `{"env":{"FROM":"legacy"}}`)
	writeFile(t, filepath.Join(configDir, ".claude.json"), `{"env":{"FROM":"ignored-global"}}`)
	writeFile(t, filepath.Join(configDir, "settings.json"), `{"env":{"FROM_USER":"cfg"}}`)

	res := mustResolve(t, Input{
		Dir:        t.TempDir(),
		Home:       t.TempDir(),
		ManagedDir: t.TempDir(),
		Environ:    []string{"CLAUDE_CONFIG_DIR=" + configDir, "PATH=/bin"},
		DiscoverGit: func(string) (string, error) {
			return "", nil
		},
	})
	got := envMap(t, res.Lines)
	assertEnv(t, got, map[string]string{
		"CLAUDE_CONFIG_DIR": configDir,
		"FROM":              "legacy",
		"FROM_USER":         "cfg",
		"PATH":              "/bin",
	})
}

func TestResolve_FiltersAndScrub(t *testing.T) {
	tests := []struct {
		name    string
		environ []string
		global  string
		user    string
		want    map[string]string
		absent  []string
	}{
		{
			name:    "scrub flag in the shell strips credentials and INPUT twins",
			environ: []string{"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1", "ANTHROPIC_API_KEY=shell-key", "INPUT_ANTHROPIC_API_KEY=input-key", "GH_TOKEN=keep", "PATH=/bin"},
			want: map[string]string{
				"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB": "1",
				"GH_TOKEN":                         "keep",
				"PATH":                             "/bin",
			},
			absent: []string{"ANTHROPIC_API_KEY", "INPUT_ANTHROPIC_API_KEY"},
		},
		{
			name:    "settings can turn scrub off",
			environ: []string{"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1", "ANTHROPIC_API_KEY=shell-key", "PATH=/bin"},
			user:    `{"env":{"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB":"0"}}`,
			want: map[string]string{
				"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB": "0",
				"ANTHROPIC_API_KEY":                "shell-key",
				"PATH":                             "/bin",
			},
		},
		{
			name:    "settings can turn scrub on and still not keep the key",
			environ: []string{"PATH=/bin"},
			user:    `{"env":{"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB":"yes","ANTHROPIC_API_KEY":"from-settings","AWS_SECRET_ACCESS_KEY":"aws"}}`,
			want: map[string]string{
				"CLAUDE_CODE_SUBPROCESS_ENV_SCRUB": "yes",
				"PATH":                             "/bin",
			},
			absent: []string{"ANTHROPIC_API_KEY", "AWS_SECRET_ACCESS_KEY"},
		},
		{
			name:    "host-managed routing blocks settings from the next overlay",
			environ: []string{"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST=1", "ANTHROPIC_MODEL=shell-model", "PATH=/bin"},
			user:    `{"env":{"ANTHROPIC_MODEL":"evil","BASH_MAX_TIMEOUT_MS":"9","VERTEX_REGION_CLAUDE_4_0_OPUS":"evil-region","vertex_region_claude_foo":"nope"}}`,
			want: map[string]string{
				"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST": "1",
				"ANTHROPIC_MODEL":                      "shell-model",
				"BASH_MAX_TIMEOUT_MS":                  "9",
				"PATH":                                 "/bin",
			},
			absent: []string{"VERTEX_REGION_CLAUDE_4_0_OPUS", "vertex_region_claude_foo"},
		},
		{
			name:    "host flag set in the same overlay does not filter that overlay",
			environ: []string{"PATH=/bin"},
			user:    `{"env":{"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST":"true","ANTHROPIC_MODEL":"from-settings"}}`,
			want: map[string]string{
				"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST": "true",
				"ANTHROPIC_MODEL":                      "from-settings",
				"PATH":                                 "/bin",
			},
		},
		{
			name:    "empty string in settings overrides the shell",
			environ: []string{"FOO=bar", "PATH=/bin"},
			user:    `{"env":{"FOO":""}}`,
			want:    map[string]string{"FOO": "", "PATH": "/bin"},
		},
		{
			name:    "numbers and bools coerce, nulls disappear",
			environ: []string{"PATH=/bin"},
			user:    `{"env":{"N":300000,"B":true,"GONE":null,"KEEP":"1"}}`,
			want: map[string]string{
				"N":    "300000",
				"B":    "true",
				"KEEP": "1",
				"PATH": "/bin",
			},
			absent: []string{"GONE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.global != "" {
				writeFile(t, filepath.Join(home, ".claude.json"), tt.global)
			}
			if tt.user != "" {
				writeFile(t, filepath.Join(home, ".claude", "settings.json"), tt.user)
			}
			res := mustResolve(t, Input{
				Dir:        t.TempDir(),
				Home:       home,
				ManagedDir: t.TempDir(),
				Environ:    tt.environ,
				DiscoverGit: func(string) (string, error) {
					return "", nil
				},
			})
			if len(res.Warnings) != 0 {
				t.Fatalf("warnings: %v", res.Warnings)
			}
			got := envMap(t, res.Lines)
			assertEnv(t, got, tt.want)
			for _, k := range tt.absent {
				if _, ok := got[k]; ok {
					t.Errorf("%s still set: %q", k, got[k])
				}
			}
		})
	}
}

func TestResolve_TunnelFilterIsTwoPhase(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude.json"), `{
		"env": {"ANTHROPIC_UNIX_SOCKET": "/tmp/sock", "FOO": "global"}
	}`)
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{
		"env": {"ANTHROPIC_API_KEY": "from-settings", "BAR": "settings"}
	}`)

	res := mustResolve(t, Input{
		Dir:        t.TempDir(),
		Home:       home,
		ManagedDir: t.TempDir(),
		Environ:    []string{"ANTHROPIC_API_KEY=shell-key", "PATH=/bin"},
		DiscoverGit: func(string) (string, error) {
			return "", nil
		},
	})
	got := envMap(t, res.Lines)
	assertEnv(t, got, map[string]string{
		"ANTHROPIC_UNIX_SOCKET": "/tmp/sock",
		"ANTHROPIC_API_KEY":     "shell-key",
		"FOO":                   "global",
		"BAR":                   "settings",
		"PATH":                  "/bin",
	})
}

func TestResolve_BadSettings(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{not json "secret-token"}`)
	proj := t.TempDir()
	writeFile(t, filepath.Join(proj, ".claude", "settings.json"), `{
		"env": {"OK": "yes", "NEST": {"a": 1}}
	}`)

	res := mustResolve(t, Input{
		Dir:        proj,
		Home:       home,
		ManagedDir: t.TempDir(),
		Environ:    []string{"PATH=/bin"},
		DiscoverGit: func(string) (string, error) {
			return "", nil
		},
	})
	got := envMap(t, res.Lines)
	assertEnv(t, got, map[string]string{"OK": "yes", "PATH": "/bin"})
	if _, ok := got["NEST"]; ok {
		t.Fatalf("nested env value applied: %v", got)
	}
	joined := strings.Join(res.Warnings, "\n")
	if strings.Contains(joined, "secret-token") {
		t.Fatalf("warning leaked file contents: %s", joined)
	}
	if !strings.Contains(joined, "invalid JSON") {
		t.Fatalf("warnings = %q, want invalid JSON", joined)
	}
	if !strings.Contains(joined, "NEST") {
		t.Fatalf("warnings = %q, want the bad key", joined)
	}
}

func TestResolve_GitRootLocalSettings(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init")
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, ".claude", "settings.local.json"), `{"env":{"FROM":"root","ROOT_ONLY":"1"}}`)
	writeFile(t, filepath.Join(sub, ".claude", "settings.json"), `{"env":{"FROM":"project","PROJECT_ONLY":"1"}}`)
	writeFile(t, filepath.Join(sub, ".claude", "settings.local.json"), `{"env":{"FROM":"cwd-local","CWD_ONLY":"1"}}`)

	res := mustResolve(t, Input{
		Dir:        sub,
		Home:       t.TempDir(),
		ManagedDir: t.TempDir(),
		Environ:    []string{"PATH=/bin"},
	})
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	got := envMap(t, res.Lines)
	assertEnv(t, got, map[string]string{
		"FROM":         "root",
		"ROOT_ONLY":    "1",
		"PROJECT_ONLY": "1",
		"CWD_ONLY":     "1",
		"PATH":         "/bin",
	})
}

func TestResolve_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "settings.local.json"), `{"env":{"FROM":"cwd"}}`)
	res := mustResolve(t, Input{
		Dir:        dir,
		Home:       t.TempDir(),
		ManagedDir: t.TempDir(),
		Environ:    []string{"PATH=/bin"},
	})
	got := envMap(t, res.Lines)
	if got["FROM"] != "cwd" {
		t.Fatalf("FROM = %q, want cwd", got["FROM"])
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
}

func mustResolve(t *testing.T, in Input) Result {
	t.Helper()
	res, err := Resolve(in)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func envMap(t *testing.T, lines []string) map[string]string {
	t.Helper()
	got := make(map[string]string, len(lines))
	for _, line := range lines {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("line %q has no =", line)
		}
		got[k] = v
	}
	return got
}

func assertEnv(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("env has %d keys, want %d\n got  %v\n want %v", len(got), len(want), got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
