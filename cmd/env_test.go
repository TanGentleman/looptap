package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestEnvCmd_RefusesUnlessDebug(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"unset", ""},
		{"zero", "0"},
		{"true", "true"},
		{"padded", "1 "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LOOPTAP_DEBUG", tc.val)
			t.Setenv("LOOPTAP_ENV_CANARY", "do-not-print")

			cmd := NewEnvCmd()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error while the debug hatch is shut")
			}
			if !strings.Contains(err.Error(), "LOOPTAP_DEBUG=1") {
				t.Errorf("error %q does not mention the hatch", err)
			}
			out := stdout.String()
			errOut := stderr.String()
			// Cobra writes usage to stdout on a RunE error. That text has no
			// KEY=VALUE lines; an env dump would.
			if strings.Contains(out, "=") {
				t.Errorf("stdout looks like an env dump (%d bytes)", len(out))
			}
			if strings.Contains(out, "do-not-print") || strings.Contains(errOut, "do-not-print") ||
				strings.Contains(out, "LOOPTAP_ENV_CANARY=") || strings.Contains(errOut, "LOOPTAP_ENV_CANARY=") {
				t.Errorf("canary leaked while the hatch was shut")
			}
		})
	}
}

func TestEnvCmd_PrintsEnvironWhenDebug(t *testing.T) {
	t.Setenv("LOOPTAP_DEBUG", "1")
	t.Setenv("LOOPTAP_ENV_CANARY", "plum")

	cmd := NewEnvCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{"LOOPTAP_DEBUG=1\n", "LOOPTAP_ENV_CANARY=plum\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout missing %q", want)
		}
	}

	// Sorted: the canary name lands after LOOPTAP_DEBUG.
	debugAt := strings.Index(got, "LOOPTAP_DEBUG=1\n")
	canaryAt := strings.Index(got, "LOOPTAP_ENV_CANARY=plum\n")
	if debugAt < 0 || canaryAt < 0 || debugAt > canaryAt {
		t.Errorf("expected LOOPTAP_DEBUG before LOOPTAP_ENV_CANARY in sorted output")
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("stdout missing trailing newline")
	}
}
