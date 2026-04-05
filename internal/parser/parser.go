package parser

import (
	"fmt"
	"os"
	"path/filepath"
)

// Parser is the interface that agent transcript parsers must implement.
type Parser interface {
	Name() string
	CanParse(path string) bool
	Parse(path string) (Session, error)
}

// all registered parsers
var parsers = []Parser{
	&ClaudeCode{},
	&Codex{},
}

// Detect returns the right parser for a file, or an error if none match.
func Detect(path string) (Parser, error) {
	for _, p := range parsers {
		if p.CanParse(path) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no parser found for %s", path)
}

// Discover walks dirs and returns parseable file paths.
func Discover(dirs []string) ([]string, error) {
	var paths []string
	for _, dir := range dirs {
		expanded := expandHome(dir)
		err := filepath.Walk(expanded, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip inaccessible paths
			}
			if info.IsDir() {
				return nil
			}
			if _, detectErr := Detect(path); detectErr == nil {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", dir, err)
		}
	}
	return paths, nil
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
