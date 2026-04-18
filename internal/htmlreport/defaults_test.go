package htmlreport

import (
	"encoding/json"
	"testing"
)

// TestDefaultOpencodeConfig_Schema locks down the shape of the embedded
// safe-default config so a careless edit to opencode.default.json fails
// loudly rather than landing on an unsuspecting user as a runtime error
// from opencode — or worse, a regression that re-opens the RCE hole.
//
// Contract is per https://opencode.ai/docs/config/ (verified against
// sst/opencode@dev packages/opencode/src/config/*.ts).
func TestDefaultOpencodeConfig_Schema(t *testing.T) {
	if len(DefaultOpencodeConfig) == 0 {
		t.Fatal("DefaultOpencodeConfig is empty — go:embed wiring broken?")
	}

	// `bash` can be a string ("allow"|"deny"|"ask") OR a pattern map;
	// everything else in this config is a plain string. Unmarshal into
	// json.RawMessage for a shape-check, then into typed structures for
	// the value-checks.
	var cfg struct {
		Schema     string                     `json:"$schema"`
		Model      string                     `json:"model"`
		Permission map[string]json.RawMessage `json:"permission"`
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

	wantAllowStr := []string{"read", "glob", "grep", "list"}
	wantDenyStr := []string{"edit", "webfetch", "websearch"}
	for _, k := range wantAllowStr {
		var v string
		if err := json.Unmarshal(cfg.Permission[k], &v); err != nil || v != "allow" {
			t.Errorf("permission.%s = %s, want \"allow\"", k, cfg.Permission[k])
		}
	}
	for _, k := range wantDenyStr {
		var v string
		if err := json.Unmarshal(cfg.Permission[k], &v); err != nil || v != "deny" {
			t.Errorf("permission.%s = %s, want \"deny\"", k, cfg.Permission[k])
		}
	}

	// bash must be a pattern map — NOT bash: "allow". This is the whole
	// point of this test: guard the security regression.
	bashRaw, ok := cfg.Permission["bash"]
	if !ok {
		t.Fatal("permission.bash missing")
	}
	var bashStr string
	if err := json.Unmarshal(bashRaw, &bashStr); err == nil {
		t.Fatalf("permission.bash is a string (%q) — the safe default must be a pattern map, not bash:allow. See 3104568700.", bashStr)
	}
	var bashMap map[string]string
	if err := json.Unmarshal(bashRaw, &bashMap); err != nil {
		t.Fatalf("permission.bash is neither string nor map: %v", err)
	}
	if bashMap["*"] != "deny" {
		t.Errorf("bash.\"*\" = %q, want \"deny\" (the fallback has to shut everything down)", bashMap["*"])
	}
	for _, pat := range []string{"git -c *", "git config *", "git submodule *", "git fetch *", "git push *", "git clone *"} {
		if bashMap[pat] != "deny" {
			t.Errorf("bash[%q] = %q, want \"deny\" (escape-hatch must stay closed)", pat, bashMap[pat])
		}
	}
	for _, pat := range []string{"git log *", "git diff *", "git rev-parse *", "git show *"} {
		if bashMap[pat] != "allow" {
			t.Errorf("bash[%q] = %q, want \"allow\" (needed for the report)", pat, bashMap[pat])
		}
	}
}

// TestSandboxOpencodeConfig_Schema confirms the sandbox variant keeps the
// write/web locks but opens bash — explicit contract so future edits don't
// accidentally ship a "sandbox" config that's safer than advertised
// (confusing) or looser than advertised (dangerous).
func TestSandboxOpencodeConfig_Schema(t *testing.T) {
	if len(SandboxOpencodeConfig) == 0 {
		t.Fatal("SandboxOpencodeConfig is empty — go:embed wiring broken?")
	}
	var cfg struct {
		Permission map[string]string `json:"permission"`
	}
	if err := json.Unmarshal(SandboxOpencodeConfig, &cfg); err != nil {
		t.Fatalf("sandbox config is not valid JSON: %v", err)
	}
	if cfg.Permission["bash"] != "allow" {
		t.Errorf("sandbox bash = %q, want \"allow\" (whole point of sandbox)", cfg.Permission["bash"])
	}
	for _, k := range []string{"edit", "webfetch", "websearch"} {
		if cfg.Permission[k] != "deny" {
			t.Errorf("sandbox %s = %q, want \"deny\" (no writes, no egress)", k, cfg.Permission[k])
		}
	}
}
