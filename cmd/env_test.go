package cmd

import (
	"bytes"
	"looptap/internal/claudeenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvCmd_PrintsClaudeEnv(t *testing.T) {
	home := t.TempDir()
	proj := t.TempDir()
	writeEnvFile(t, filepath.Join(home, ".claude", "settings.json"), `{"env":{"FROM_USER":"user"}}`)
	writeEnvFile(t, filepath.Join(proj, ".claude", "settings.json"), `{"env":{"FROM_PROJECT":"proj","FROM_USER":"overridden"}}`)

	c := newEnvCmd(claudeenv.Input{
		Home:       home,
		ManagedDir: t.TempDir(),
		Environ:    []string{"PATH=/usr/bin", "ONLY_SHELL=yes"},
		DiscoverGit: func(string) (string, error) {
			return "", nil
		},
	})
	c.SetArgs([]string{"--dir", proj})
	var stdout, stderr bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(&stderr)
	if err := c.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: %s", stderr.String())
	}
	got := stdout.String()
	for _, line := range []string{
		"FROM_PROJECT=proj",
		"FROM_USER=overridden",
		"ONLY_SHELL=yes",
		"PATH=/usr/bin",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("stdout missing %q:\n%s", line, got)
		}
	}
}

func TestEnvCmd_WarnsOnBadJSON(t *testing.T) {
	home := t.TempDir()
	writeEnvFile(t, filepath.Join(home, ".claude", "settings.json"), `{`)

	c := newEnvCmd(claudeenv.Input{
		Home:       home,
		ManagedDir: t.TempDir(),
		Environ:    []string{"PATH=/usr/bin"},
		DiscoverGit: func(string) (string, error) {
			return "", nil
		},
	})
	c.SetArgs([]string{"--dir", t.TempDir()})
	var stdout, stderr bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(&stderr)
	if err := c.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(stdout.String(), "PATH=/usr/bin\n") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid JSON") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func writeEnvFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
