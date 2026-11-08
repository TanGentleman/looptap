package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultSourcePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	tests := []struct {
		name      string
		claudeDir string
		codexHome string
		want      []string
	}{
		{"default", "", "", []string{
			filepath.Join(home, ".claude", "projects"),
			filepath.Join(home, ".codex", "sessions"),
			filepath.Join(home, ".codex", "archived_sessions"),
		}},
		{"env overrides", "/srv/claude", "/srv/codex", []string{
			filepath.Join("/srv/claude", "projects"),
			filepath.Join("/srv/codex", "sessions"),
			filepath.Join("/srv/codex", "archived_sessions"),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tt.claudeDir)
			t.Setenv("CODEX_HOME", tt.codexHome)
			got := DefaultSourcePaths()
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("DefaultSourcePaths() = %v, want %v", got, tt.want)
			}
		})
	}
}
