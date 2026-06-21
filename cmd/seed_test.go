package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"looptap/internal/rule"
)

// runSeed drives the seed-contract-fixture command against a fresh db path and
// returns that path plus stdout.
func runSeed(t *testing.T, args ...string) (dbPath, stdout string) {
	t.Helper()
	dbPath = filepath.Join(t.TempDir(), "seed.db")
	dbp := dbPath
	cmd := NewSeedContractFixtureCmd(&dbp)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v (stderr: %s)", args, err, errBuf.String())
	}
	return dbPath, out.String()
}

// bundleFrom seeds, then drives the real patterns command over the same db to
// get the live bundle a consumer would capture.
func bundleFrom(t *testing.T, leaky bool) rule.Bundle {
	t.Helper()
	args := []string{}
	if leaky {
		args = append(args, "--leaky")
	}
	dbPath, _ := runSeed(t, args...)

	dbp := dbPath
	cmd := NewPatternsCmd(&dbp)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("patterns: %v", err)
	}

	var b rule.Bundle
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		t.Fatalf("bundle isn't valid json: %v\n%s", err, out.String())
	}
	return b
}

func TestSeedContractFixtureCmd(t *testing.T) {
	for _, tc := range []struct {
		name        string
		leaky       bool
		wantRedacts bool
	}{
		{"default", false, false},
		{"leaky", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := bundleFrom(t, tc.leaky)

			// Only the 6-session ENOENT cluster clears the default gate of 5.
			if len(b.Cards) != 1 {
				t.Fatalf("got %d cards, want 1: %+v", len(b.Cards), b.Cards)
			}
			card := b.Cards[0]
			if card.Pattern.ErrorClass != "ENOENT" {
				t.Errorf("card error class = %q, want ENOENT", card.Pattern.ErrorClass)
			}
			if card.Pattern.SessionCount != 6 {
				t.Errorf("session count = %d, want 6 (the inline seeder's 7 was a drift)", card.Pattern.SessionCount)
			}

			var redacted int
			for _, ev := range card.Evidence {
				if strings.Contains(ev.Excerpt, "sk-ant") {
					t.Errorf("raw secret leaked into the bundle: %q", ev.Excerpt)
				}
				if ev.Redactions > 0 {
					redacted++
					if !strings.Contains(ev.Excerpt, "[REDACTED]") {
						t.Errorf("redaction count without a placeholder: %q", ev.Excerpt)
					}
				}
			}

			switch {
			case tc.wantRedacts && redacted == 0:
				t.Error("--leaky bundle carried no redacted evidence")
			case !tc.wantRedacts && redacted != 0:
				t.Errorf("default bundle redacted %d rows, want 0", redacted)
			}
		})
	}
}

func TestSeedContractFixtureCmd_Stdout(t *testing.T) {
	_, out := runSeed(t, "--leaky")
	if !strings.Contains(out, "leaky=true") {
		t.Errorf("stdout should report the leaky flag: %q", out)
	}
}
