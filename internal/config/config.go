package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config represents the ~/.looptap/config.toml configuration.
type Config struct {
	Database DatabaseConfig `toml:"database"`
	Sources  SourcesConfig  `toml:"sources"`
	Signals  SignalsConfig  `toml:"signals"`
	Phrases  PhrasesConfig  `toml:"phrases"`
	Advise   AdviseConfig   `toml:"advise"`
}

type AdviseConfig struct {
	APIKey string `toml:"api_key"` // fallback; prefer GOOGLE_API_KEY env
	Model  string `toml:"model"`   // default gemini-3.1-flash-lite-preview
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type SourcesConfig struct {
	Paths []string `toml:"paths"`
}

type SignalsConfig struct {
	StagnationSimilarity float64 `toml:"stagnation_similarity"`
	StagnationTurnFactor float64 `toml:"stagnation_turn_factor"`
	LoopWindow           int     `toml:"loop_window"`
	LoopMinRepeats       int     `toml:"loop_min_repeats"`
}

type PhrasesConfig struct {
	Misalignment      string `toml:"misalignment"`
	MisalignmentExtra string `toml:"misalignment_extra"`
	Disengagement     string `toml:"disengagement"`
	DisengagementExtra string `toml:"disengagement_extra"`
	Satisfaction      string `toml:"satisfaction"`
	SatisfactionExtra string `toml:"satisfaction_extra"`
}

// DefaultDBPath returns the default database path.
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "looptap.db"
	}
	return filepath.Join(home, ".looptap", "looptap.db")
}

// DefaultSourcePaths returns the default transcript directories.
func DefaultSourcePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".claude", "projects"),
		filepath.Join(home, ".codex", "sessions"),
	}
}

// Load reads the config from ~/.looptap/config.toml.
// Returns defaults if the file doesn't exist.
func Load() (*Config, error) {
	cfg := &Config{
		Database: DatabaseConfig{Path: DefaultDBPath()},
		Sources:  SourcesConfig{Paths: DefaultSourcePaths()},
		Signals: SignalsConfig{
			StagnationSimilarity: 0.8,
			StagnationTurnFactor: 2.0,
			LoopWindow:           6,
			LoopMinRepeats:       3,
		},
		Advise: AdviseConfig{
			Model: "gemini-3.1-flash-lite-preview",
		},
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	path := filepath.Join(home, ".looptap", "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}

	// Expand ~ in database path
	if len(cfg.Database.Path) > 0 && cfg.Database.Path[0] == '~' {
		cfg.Database.Path = filepath.Join(home, cfg.Database.Path[1:])
	}

	return cfg, nil
}
