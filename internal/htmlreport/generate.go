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

// Runner shells out to (or fakes) a claude print-mode invocation. Given a
// working directory and the args to pass after the binary name, it returns
// stdout. Keeping it as a function type makes test stubbing trivial.
type Runner func(ctx context.Context, dir string, args []string) (string, error)

// Generate asks claude, running headless in r.RepoPath, to produce a
// self-contained HTML report describing the branch. A nil runner falls back
// to the real `claude` binary on PATH (override with LOOPTAP_CLAUDE_BIN).
func Generate(ctx context.Context, r *Resolved, runner Runner) (string, error) {
	if r == nil {
		return "", fmt.Errorf("generate: nil resolved settings")
	}
	if runner == nil {
		runner = defaultRunner
	}

	args := buildClaudeArgs(r)
	out, err := runner(ctx, r.RepoPath, args)
	if err != nil {
		return "", err
	}

	html := stripFences(out)
	if !looksLikeHTML(html) {
		return "", fmt.Errorf("claude returned %d bytes but no HTML document — check your prompt or permissions", len(out))
	}
	return html, nil
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

// defaultRunner invokes the real `claude` binary. Honored env vars:
//
//	LOOPTAP_CLAUDE_BIN — override the binary name/path
func defaultRunner(ctx context.Context, dir string, args []string) (string, error) {
	bin := "claude"
	if b := os.Getenv("LOOPTAP_CLAUDE_BIN"); b != "" {
		bin = b
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s -p failed: %w\nstderr: %s", bin, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// looksLikeHTML is a cheap sanity check that we got an HTML document back.
func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype html")
}

// stripFences peels off a ```html ... ``` wrapper if claude added one despite
// being told not to. Better to be forgiving than fail on a cosmetic nit.
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
