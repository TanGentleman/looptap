package cmd

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestEnvCmd_LockedWritesNothing(t *testing.T) {
	t.Setenv("LOOPTAP_DEBUG", "")

	c := NewEnvCmd()
	var out, errOut bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errOut)
	c.SetArgs([]string{})

	if err := c.Execute(); err == nil {
		t.Fatal("expected refusal")
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
}

func TestEnvCmd_DebugPrintsProcessEnv(t *testing.T) {
	t.Setenv("LOOPTAP_DEBUG", "1")
	t.Setenv("LOOPTAP_ENV_PROBE", "present")

	c := NewEnvCmd()
	var out, errOut bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errOut)
	c.SetArgs([]string{})

	if err := c.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr: %s", err, errOut.String())
	}

	got := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if !sort.StringsAreSorted(got) {
		t.Fatal("output is not sorted")
	}

	want := append([]string(nil), os.Environ()...)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("printed %d vars, process has %d", len(got), len(want))
	}
	if !strings.Contains(out.String(), "LOOPTAP_ENV_PROBE=present\n") {
		t.Error("probe variable missing from output")
	}
	if !strings.Contains(out.String(), "LOOPTAP_DEBUG=1\n") {
		t.Error("debug gate missing from output")
	}
}
