package htmlreport

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Prompts live in prompt.go — systemAppend and buildPrompt.

// Runner shells out to (or fakes) an agent invocation. Given a working
// directory and the args to pass after the binary name, it returns stdout.
// The binary and any agent-specific env (e.g. OPENCODE_CONFIG) are decided
// inside the runner so the seam stays dumb and test-friendly.
type Runner func(ctx context.Context, dir string, args []string) (string, error)

// Generate asks the configured agent, running headless in r.RepoPath, to
// produce a self-contained HTML report describing the branch. A nil runner
// falls back to the real binary on PATH for the selected agent.
func Generate(ctx context.Context, r *Resolved, runner Runner) (string, error) {
	if r == nil {
		return "", fmt.Errorf("generate: nil resolved settings")
	}
	if runner == nil {
		runner = defaultRunnerFor(r)
	}

	args := buildArgsFor(r)
	out, err := runner(ctx, r.RepoPath, args)
	if err != nil {
		return "", err
	}

	html := stripFences(out)
	if !looksLikeHTML(html) {
		return "", fmt.Errorf("%s returned %d bytes but no HTML document — check your prompt or permissions", r.Agent, len(out))
	}
	return html, nil
}

// buildArgsFor dispatches to the right arg builder for the agent.
func buildArgsFor(r *Resolved) []string {
	if r.Agent == AgentOpencode {
		return buildOpencodeArgs(r)
	}
	return buildClaudeArgs(r)
}

// buildClaudeArgs assembles the flags for `claude -p`. Read-only toolset,
// permissions bypassed (we're running in a scratch subprocess), a hard turn
// cap so a runaway agent can't burn the afternoon.
func buildClaudeArgs(r *Resolved) []string {
	return []string{
		"-p", buildPrompt(r),
		"--output-format", "text",
		"--permission-mode", "bypassPermissions",
		"--allowedTools", "Bash,Read,Glob,Grep",
		"--append-system-prompt", systemAppend,
		"--max-turns", "40",
	}
}

// buildOpencodeArgs assembles the flags for `opencode run`. Allowed tools,
// model, and system prompt come from the user-supplied config file (plumbed
// in via OPENCODE_CONFIG in the runner). `--system` overrides rather than
// appends on opencode, so we fold the strict HTML-only instruction into the
// user prompt instead of clobbering the config's system prompt.
func buildOpencodeArgs(r *Resolved) []string {
	prompt := systemAppend + "\n\n" + buildPrompt(r)
	return []string{
		"run", prompt,
		"--dangerously-skip-permissions",
	}
}

// defaultRunnerFor returns the real-binary runner appropriate for the agent.
// Tests inject their own Runner and bypass this path entirely.
func defaultRunnerFor(r *Resolved) Runner {
	if r.Agent == AgentOpencode {
		cfgPath := r.OpencodeConfigPath
		return func(ctx context.Context, dir string, args []string) (string, error) {
			return runOpencode(ctx, dir, args, cfgPath)
		}
	}
	return runClaude
}

// runClaude invokes the real `claude` binary.
//
// Honored env vars:
//
//	LOOPTAP_CLAUDE_BIN — override the binary name/path
func runClaude(ctx context.Context, dir string, args []string) (string, error) {
	bin := "claude"
	if b := os.Getenv("LOOPTAP_CLAUDE_BIN"); b != "" {
		bin = b
	}
	return runBin(ctx, bin, dir, args, nil)
}

// runOpencode invokes the real `opencode` binary with OPENCODE_CONFIG set
// to the user-supplied JSON config. An empty cfgPath means "use the embedded
// DefaultOpencodeConfig" — we materialize it to a tempfile for the duration
// of this call.
//
// Honored env vars:
//
//	LOOPTAP_OPENCODE_BIN — override the binary name/path
func runOpencode(ctx context.Context, dir string, args []string, cfgPath string) (string, error) {
	bin := "opencode"
	if b := os.Getenv("LOOPTAP_OPENCODE_BIN"); b != "" {
		bin = b
	}

	if cfgPath == "" {
		f, err := os.CreateTemp("", "looptap-opencode-*.json")
		if err != nil {
			return "", fmt.Errorf("tempfile for default opencode config: %w", err)
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(DefaultOpencodeConfig); err != nil {
			f.Close()
			return "", fmt.Errorf("writing default opencode config: %w", err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("closing default opencode config: %w", err)
		}
		cfgPath = f.Name()
	}

	env := append(os.Environ(), "OPENCODE_CONFIG="+cfgPath)
	return runBin(ctx, bin, dir, args, env)
}

// runBin is the shared exec wrapper. env==nil means inherit from the parent.
func runBin(ctx context.Context, bin, dir string, args, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %w\nstderr: %s", bin, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// looksLikeHTML is a cheap sanity check that we got an HTML document back.
func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype html")
}

// stripFences peels off a ```html ... ``` wrapper if the agent added one
// despite being told not to. Better to be forgiving than fail on a cosmetic
// nit.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line (```html or just ```).
	nl := strings.Index(s, "\n")
	if nl == -1 {
		return s
	}
	s = s[nl+1:]
	// Drop the closing fence if present.
	if idx := strings.LastIndex(s, "```"); idx != -1 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
