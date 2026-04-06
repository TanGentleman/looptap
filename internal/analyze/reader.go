package analyze

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultClaudeMDPath returns ~/.claude/CLAUDE.md.
func DefaultClaudeMDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "CLAUDE.md"), nil
}

// ReadFile reads the target file and returns its contents.
// Returns a clear error if the file doesn't exist — that's a user-facing message.
func ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("no file at %s — nothing to analyze", path)
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%s is empty — nothing to analyze", path)
	}
	return string(data), nil
}
