package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSourcePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	tests := []struct {
		name      string
		configDir string
		want      string
	}{
		{"default", "", filepath.Join(home, ".claude", "projects")},
		{"CLAUDE_CONFIG_DIR", "/srv/claude", filepath.Join("/srv/claude", "projects")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tt.configDir)
			got := DefaultSourcePaths()
			if len(got) == 0 || got[0] != tt.want {
				t.Errorf("DefaultSourcePaths()[0] = %v, want %s", got, tt.want)
			}
		})
	}
}
