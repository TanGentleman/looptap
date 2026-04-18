package htmlreport

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// recordingRunner captures whatever the caller asked it to run, so tests
// can assert on the wiring without actually invoking a real agent.
type recordingRunner struct {
	out     string
	err     error
	gotDir  string
	gotArgs []string
	calls   int
}

func (r *recordingRunner) Run(ctx context.Context, dir string, args []string) (string, error) {
	r.calls++
	r.gotDir = dir
	r.gotArgs = args
	if r.err != nil {
		return "", r.err
	}
	return r.out, nil
}

func (r *recordingRunner) asRunner() Runner {
	return r.Run
}

func TestGenerate_PassesArgsAndDir(t *testing.T) {
	r := &Resolved{
		RepoPath:   "/tmp/myrepo",
		Branch:     "feature/x",
		BranchMode: BranchCustom,
		Agent:      AgentClaude,
	}
	rec := &recordingRunner{out: "<!doctype html><html><body>hi</body></html>"}

	html, err := Generate(context.Background(), r, rec.asRunner())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(html, "<body>hi</body>") {
		t.Errorf("unexpected output: %s", html)
	}
	if rec.gotDir != "/tmp/myrepo" {
		t.Errorf("cwd = %q, want /tmp/myrepo", rec.gotDir)
	}

	joined := strings.Join(rec.gotArgs, " ")
	wants := []string{
		"-p",
		"feature/x",
		"--output-format text",
		"--permission-mode bypassPermissions",
		"--allowedTools Bash,Read,Glob,Grep",
		"--append-system-prompt",
		"--max-turns 40",
	}
	for _, w := range wants {
		if !strings.Contains(joined, w) {
			t.Errorf("args missing %q\nfull: %s", w, joined)
		}
	}
}

func TestGenerate_OpencodeArgs(t *testing.T) {
	// Default (non-sandbox): NO --dangerously-skip-permissions. This is
	// the security contract — the flag only ships when the caller opts in.
	r := &Resolved{
		RepoPath:           "/tmp/myrepo",
		Branch:             "feature/x",
		BranchMode:         BranchCustom,
		Agent:              AgentOpencode,
		OpencodeConfigPath: "/tmp/opencode.json",
	}
	rec := &recordingRunner{out: "<!doctype html><html><body>oc</body></html>"}

	html, err := Generate(context.Background(), r, rec.asRunner())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(html, "<body>oc</body>") {
		t.Errorf("unexpected output: %s", html)
	}

	if len(rec.gotArgs) == 0 || rec.gotArgs[0] != "run" {
		t.Fatalf("expected first arg to be 'run', got %v", rec.gotArgs)
	}

	joined := strings.Join(rec.gotArgs, " ")
	if strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Errorf("non-sandbox opencode must NOT include --dangerously-skip-permissions: %s", joined)
	}
	// The strict HTML-only instruction must be folded into the user prompt,
	// since opencode's --system overrides rather than appends.
	if !strings.Contains(rec.gotArgs[1], "<!doctype html>") {
		t.Errorf("opencode prompt missing systemAppend text: %s", rec.gotArgs[1])
	}
	if !strings.Contains(rec.gotArgs[1], "feature/x") {
		t.Errorf("opencode prompt missing branch name: %s", rec.gotArgs[1])
	}
	// Claude-only flags must NOT leak into opencode invocations.
	for _, bad := range []string{"-p ", "--append-system-prompt", "--max-turns", "--allowedTools", "--permission-mode"} {
		if strings.Contains(joined, bad) {
			t.Errorf("opencode args contain claude-only flag %q: %s", bad, joined)
		}
	}
}

func TestGenerate_OpencodeSandboxArgs(t *testing.T) {
	// Sandbox mode: --dangerously-skip-permissions DOES ship. This is the
	// explicit-opt-in knob for CI runners and disposable containers.
	r := &Resolved{
		RepoPath:           "/tmp/myrepo",
		Branch:             "feature/x",
		BranchMode:         BranchCustom,
		Agent:              AgentOpencode,
		OpencodeConfigPath: "/tmp/opencode.json",
		IsSandbox:          true,
	}
	rec := &recordingRunner{out: "<!doctype html><html></html>"}
	if _, err := Generate(context.Background(), r, rec.asRunner()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	joined := strings.Join(rec.gotArgs, " ")
	if !strings.Contains(joined, "--dangerously-skip-permissions") {
		t.Errorf("sandbox opencode must include --dangerously-skip-permissions: %s", joined)
	}
}

func TestGenerate_StripsFences(t *testing.T) {
	r := &Resolved{RepoPath: "/tmp/r", Branch: "main", BranchMode: BranchCurrent, Agent: AgentClaude}
	rec := &recordingRunner{out: "```html\n<!doctype html><html></html>\n```\n"}

	html, err := Generate(context.Background(), r, rec.asRunner())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(html, "```") {
		t.Errorf("fences not stripped: %q", html)
	}
	if !strings.HasPrefix(html, "<!doctype html>") {
		t.Errorf("expected html doctype, got: %q", html)
	}
}

func TestGenerate_RejectsNonHTML(t *testing.T) {
	r := &Resolved{RepoPath: "/tmp/r", Branch: "main", BranchMode: BranchCurrent, Agent: AgentClaude}
	rec := &recordingRunner{out: "I can't help with that."}
	if _, err := Generate(context.Background(), r, rec.asRunner()); err == nil {
		t.Error("expected error for non-HTML output")
	}
}

func TestGenerate_PropagatesRunnerError(t *testing.T) {
	r := &Resolved{RepoPath: "/tmp/r", Branch: "main", BranchMode: BranchCurrent, Agent: AgentClaude}
	rec := &recordingRunner{err: errors.New("boom")}
	_, err := Generate(context.Background(), r, rec.asRunner())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected boom error, got %v", err)
	}
}

func TestGenerate_NilSettings(t *testing.T) {
	if _, err := Generate(context.Background(), nil, (&recordingRunner{}).asRunner()); err == nil {
		t.Error("expected error for nil settings")
	}
}

func TestBuildPromptContainsEssentials(t *testing.T) {
	r := &Resolved{RepoPath: "/tmp/r", Branch: "feature/x", BranchMode: BranchCustom}
	p := strings.ToLower(buildPrompt(r))
	for _, want := range []string{"feature/x", "<!doctype html>", "narrative", "review"} {
		if !strings.Contains(p, strings.ToLower(want)) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestStripFences(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"no fences", "<html></html>", "<html></html>"},
		{"html fence", "```html\n<html></html>\n```", "<html></html>"},
		{"bare fence", "```\n<html></html>\n```", "<html></html>"},
		{"trims whitespace", "  <html></html>  ", "<html></html>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripFences(tt.in); got != tt.want {
				t.Errorf("stripFences(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
