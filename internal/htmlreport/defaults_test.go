package htmlreport

import (
	"encoding/json"
	"testing"
)

// TestDefaultOpencodeConfig_Schema locks down the shape of the embedded
// config so a careless edit to opencode.default.json fails loudly rather
// than landing on an unsuspecting user as a runtime error from opencode.
//
// Contract is per https://opencode.ai/docs/config/ (verified against
// sst/opencode@dev packages/opencode/src/config/*.ts).
func TestDefaultOpencodeConfig_Schema(t *testing.T) {
	if len(DefaultOpencodeConfig) == 0 {
		t.Fatal("DefaultOpencodeConfig is empty — go:embed wiring broken?")
	}

	var cfg struct {
		Schema     string            `json:"$schema"`
		Model      string            `json:"model"`
		Permission map[string]string `json:"permission"`
	}
	if err := json.Unmarshal(DefaultOpencodeConfig, &cfg); err != nil {
		t.Fatalf("default config is not valid JSON: %v", err)
	}

	if cfg.Schema != "https://opencode.ai/config.json" {
		t.Errorf("$schema = %q, want https://opencode.ai/config.json", cfg.Schema)
	}
	if cfg.Model == "" {
		t.Error("model is empty")
	}

	// Read-only-ish shape: we want code inspection + git, not writes or web.
	wantAllow := []string{"read", "glob", "grep", "list", "bash"}
	wantDeny := []string{"edit", "webfetch", "websearch"}
	for _, k := range wantAllow {
		if v := cfg.Permission[k]; v != "allow" {
			t.Errorf("permission.%s = %q, want allow", k, v)
		}
	}
	for _, k := range wantDeny {
		if v := cfg.Permission[k]; v != "deny" {
			t.Errorf("permission.%s = %q, want deny", k, v)
		}
	}

	// Every permission action must be one of the three valid verbs.
	for k, v := range cfg.Permission {
		switch v {
		case "allow", "deny", "ask":
		default:
			t.Errorf("permission.%s = %q, want allow|deny|ask", k, v)
		}
	}
}
