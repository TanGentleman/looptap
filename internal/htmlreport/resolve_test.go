package htmlreport

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBranchFlag(t *testing.T) {
	tests := []struct {
		in       string
		wantMode BranchMode
		wantName string
	}{
		{"", BranchCurrent, ""},
		{"current", BranchCurrent, ""},
		{"CURRENT", BranchCurrent, ""},
		{"  current  ", BranchCurrent, ""},
		{"default", BranchDefault, ""},
		{"Default", BranchDefault, ""},
		{"feature/foo", BranchCustom, "feature/foo"},
		{"  my-branch  ", BranchCustom, "my-branch"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			mode, name := ParseBranchFlag(tt.in)
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestResolved_Summary(t *testing.T) {
	r := &Resolved{RepoPath: "/tmp/r", Branch: "main", BranchMode: BranchCurrent}
	s := r.Summary()
	for _, want := range []string{"/tmp/r", "main", "current"} {
		if !strings.Contains(s, want) {
			t.Errorf("summary missing %q: %s", want, s)
		}
	}
}

func TestResolve_RepoErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if _, err := Resolve(HTMLSettings{RepoPath: missing, BranchMode: BranchCurrent}); err == nil {
		t.Error("expected error for missing path")
	} else if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("wrong error: %v", err)
	}

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(HTMLSettings{RepoPath: file, BranchMode: BranchCurrent}); err == nil {
		t.Error("expected error for file-not-dir")
	} else if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("wrong error: %v", err)
	}

	notARepo := t.TempDir()
	if _, err := Resolve(HTMLSettings{RepoPath: notARepo, BranchMode: BranchCurrent}); err == nil {
		t.Error("expected error for non-git dir")
	} else if !strings.Contains(err.Error(), "not a git repo") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestResolve_Repo(t *testing.T) {
	repo := initTestRepo(t)

	r, err := Resolve(HTMLSettings{RepoPath: repo, BranchMode: BranchCurrent})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	// git rev-parse --show-toplevel may canonicalize the path (e.g. resolving
	// /var vs /private/var on macOS), so compare by basename.
	if filepath.Base(r.RepoPath) != filepath.Base(repo) {
		t.Errorf("RepoPath = %q, want basename %q", r.RepoPath, filepath.Base(repo))
	}
	if r.Branch != "main" {
		t.Errorf("Branch = %q, want main", r.Branch)
	}
	if r.BranchMode != BranchCurrent {
		t.Errorf("BranchMode = %q, want current", r.BranchMode)
	}
}

func TestResolve_EmptyModeDefaultsToCurrent(t *testing.T) {
	repo := initTestRepo(t)
	r, err := Resolve(HTMLSettings{RepoPath: repo})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.BranchMode != BranchCurrent {
		t.Errorf("BranchMode = %q, want current", r.BranchMode)
	}
	if r.Branch != "main" {
		t.Errorf("Branch = %q, want main", r.Branch)
	}
}

func TestResolve_BranchModes(t *testing.T) {
	repo := initTestRepo(t)
	runOrFail(t, repo, "git", "branch", "feature/x")

	tests := []struct {
		name    string
		mode    BranchMode
		branch  string
		want    string
		wantErr string
	}{
		{"current resolves HEAD", BranchCurrent, "", "main", ""},
		{"default falls back to main", BranchDefault, "", "main", ""},
		{"custom resolves existing branch", BranchCustom, "feature/x", "feature/x", ""},
		{"custom rejects missing branch", BranchCustom, "nope", "", "not found"},
		{"custom rejects empty name", BranchCustom, "", "", "requires a branch name"},
		{"unknown mode errors", BranchMode("wat"), "", "", "unknown branch mode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Resolve(HTMLSettings{
				RepoPath:   repo,
				BranchMode: tt.mode,
				BranchName: tt.branch,
			})
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q missing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r.Branch != tt.want {
				t.Errorf("branch = %q, want %q", r.Branch, tt.want)
			}
		})
	}
}

// initTestRepo creates a git repo with an initial commit on branch `main`.
// Skips the test if git isn't installed.
func initTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runOrFail(t, dir, "git", "init", "-q", "-b", "main")
	runOrFail(t, dir, "git", "config", "user.email", "test@example.com")
	runOrFail(t, dir, "git", "config", "user.name", "Test")
	runOrFail(t, dir, "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runOrFail(t, dir, "git", "add", ".")
	runOrFail(t, dir, "git", "commit", "-q", "-m", "initial")
	return dir
}

func runOrFail(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}
