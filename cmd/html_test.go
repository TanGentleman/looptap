package cmd

import (
	"bytes"
	"context"
	"looptap/internal/htmlreport"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner stands in for the real agent subprocess in cmd tests. It lets
// us verify the command path end-to-end without touching a real binary.
type fakeRunner struct {
	html    string
	err     error
	calls   int
	gotArgs []string
}

func (f *fakeRunner) runner() htmlreport.Runner {
	return func(ctx context.Context, dir string, args []string) (string, error) {
		f.calls++
		f.gotArgs = args
		if f.err != nil {
			return "", f.err
		}
		return f.html, nil
	}
}

func TestHTMLCmd_ForceWritesFile(t *testing.T) {
	repo := initRepoForCmdTest(t)
	out := filepath.Join(t.TempDir(), "report.html")

	fake := &fakeRunner{html: "<!doctype html><html><body>hi main</body></html>"}
	cmd := newHTMLCmd(fake.runner())
	cmd.SetArgs([]string{"--repo", repo, "--branch", "current", "--output", out, "--force"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if !strings.Contains(string(body), "<body>hi main</body>") {
		t.Errorf("report missing expected body: %s", body)
	}
	stdout := buf.String()
	if !strings.Contains(stdout, "--force") {
		t.Errorf("stdout missing --force notice: %s", stdout)
	}
	if !strings.Contains(stdout, "wrote "+out) {
		t.Errorf("stdout missing write confirmation: %s", stdout)
	}
	if !strings.Contains(stdout, "branch: main") {
		t.Errorf("stdout missing resolved summary: %s", stdout)
	}
	if fake.calls != 1 {
		t.Errorf("runner calls = %d, want 1", fake.calls)
	}
}

func TestHTMLCmd_ConfirmNoAborts(t *testing.T) {
	repo := initRepoForCmdTest(t)
	out := filepath.Join(t.TempDir(), "report.html")
	fake := &fakeRunner{html: "<!doctype html><html></html>"}

	cmd := newHTMLCmd(fake.runner())
	cmd.SetArgs([]string{"--repo", repo, "--output", out})
	cmd.SetIn(strings.NewReader("n\n"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Aborted") {
		t.Errorf("expected abort message, got %q", buf.String())
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("expected no output file after abort, got err=%v", err)
	}
	if fake.calls != 0 {
		t.Errorf("runner should not have been called, got %d", fake.calls)
	}
}

func TestHTMLCmd_ConfirmYesProceeds(t *testing.T) {
	repo := initRepoForCmdTest(t)
	out := filepath.Join(t.TempDir(), "report.html")
	fake := &fakeRunner{html: "<!doctype html><html></html>"}

	cmd := newHTMLCmd(fake.runner())
	cmd.SetArgs([]string{"--repo", repo, "--output", out})
	cmd.SetIn(strings.NewReader("y\n"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected output file, got err=%v", err)
	}
	if fake.calls != 1 {
		t.Errorf("runner calls = %d, want 1", fake.calls)
	}
}

func TestHTMLCmd_OpencodeAgent(t *testing.T) {
	repo := initRepoForCmdTest(t)
	cfg := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(cfg, []byte(`{"model":"anthropic/claude-sonnet-4-20250514"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeRunner{html: "<!doctype html><html><body>oc</body></html>"}
	cmd := newHTMLCmd(fake.runner())
	cmd.SetArgs([]string{
		"--repo", repo,
		"--agent", "opencode",
		"--opencode-config", cfg,
		"--force",
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	stdout := buf.String()
	if !strings.Contains(stdout, "agent:  opencode") {
		t.Errorf("stdout missing opencode agent summary: %s", stdout)
	}
	if !strings.Contains(stdout, cfg) {
		t.Errorf("stdout missing config path: %s", stdout)
	}
	if !strings.Contains(stdout, "Asking opencode") {
		t.Errorf("stdout missing 'Asking opencode' line: %s", stdout)
	}
	if fake.calls != 1 {
		t.Errorf("runner calls = %d, want 1", fake.calls)
	}
	if len(fake.gotArgs) == 0 || fake.gotArgs[0] != "run" {
		t.Errorf("expected first arg to be 'run', got %v", fake.gotArgs)
	}
}

func TestHTMLCmd_OpencodeMissingConfig(t *testing.T) {
	repo := initRepoForCmdTest(t)
	fake := &fakeRunner{html: "<!doctype html><html></html>"}
	cmd := newHTMLCmd(fake.runner())
	cmd.SetArgs([]string{"--repo", repo, "--agent", "opencode", "--force"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --opencode-config missing")
	}
	if !strings.Contains(err.Error(), "opencode-config") {
		t.Errorf("wrong error: %v", err)
	}
	if fake.calls != 0 {
		t.Errorf("runner should not have been called, got %d", fake.calls)
	}
}

func TestHTMLCmd_InvalidRepo(t *testing.T) {
	fake := &fakeRunner{html: "<!doctype html><html></html>"}
	cmd := newHTMLCmd(fake.runner())
	cmd.SetArgs([]string{"--repo", filepath.Join(t.TempDir(), "nope"), "--force"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("wrong error: %v", err)
	}
	if fake.calls != 0 {
		t.Errorf("runner should not have been called on bad repo, got %d", fake.calls)
	}
}

func TestConfirmHelper(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"\n", false},
		{"maybe\n", false},
		{"", false}, // EOF
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var out bytes.Buffer
			got, err := confirm(strings.NewReader(tt.in), &out, "? ")
			if err != nil {
				t.Fatalf("confirm: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if !strings.Contains(out.String(), "? ") {
				t.Errorf("prompt not written: %q", out.String())
			}
		})
	}
}

// initRepoForCmdTest creates a real git repo and skips if git is missing.
func initRepoForCmdTest(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	run := func(name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}
	run("git", "init", "-q", "-b", "main")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	run("git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", ".")
	run("git", "commit", "-q", "-m", "initial")
	return repo
}
