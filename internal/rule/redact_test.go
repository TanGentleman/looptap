package rule

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantCount int
		wantHas   string // substring that must survive (or "" to skip)
		wantGone  string // substring that must NOT survive (or "" to skip)
	}{
		{
			name:      "openai key",
			in:        "export OPENAI_API_KEY then call sk-abc123DEF456ghi789jkl",
			wantCount: 1, // only the key; the bare name has no =value to catch
			wantGone:  "sk-abc123DEF456ghi789jkl",
		},
		{
			name:      "github token",
			in:        "remote: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
			wantCount: 1,
			wantGone:  "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789",
		},
		{
			name:      "aws access key id",
			in:        "using AKIAIOSFODNN7EXAMPLE in the call",
			wantCount: 1,
			wantGone:  "AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:      "bearer header keeps prefix",
			in:        "Authorization: Bearer eyJhbGci.payload.sig",
			wantCount: 1,
			wantHas:   "Authorization: Bearer [REDACTED]",
			wantGone:  "eyJhbGci.payload.sig",
		},
		{
			name:      "secret env assignment",
			in:        "MY_SECRET_TOKEN=hunter2supersecret npm run deploy",
			wantCount: 1,
			wantHas:   "MY_SECRET_TOKEN=[REDACTED]",
			wantGone:  "hunter2supersecret",
		},
		{
			name:      "quoted secret env assignment",
			in:        `API_KEY="hunter2-is-my-password-value"`,
			wantCount: 1,
			wantHas:   "API_KEY=[REDACTED]",
		},
		{
			name:      "ordinary error text passes through untouched",
			in:        "cd packages/api: No such file or directory (exit code 1), FOO=bar",
			wantCount: 0,
			wantHas:   "No such file or directory",
		},
		{
			name:      "clean line is identity",
			in:        "npm test failed: 3 assertions, ECONNREFUSED 127.0.0.1:5432",
			wantCount: 0,
			wantHas:   "ECONNREFUSED 127.0.0.1:5432",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, n := Redact(tc.in)
			if tc.wantGone != "" && strings.Contains(got, tc.wantGone) {
				t.Errorf("secret survived: %q still in %q", tc.wantGone, got)
			}
			if tc.wantHas != "" && !strings.Contains(got, tc.wantHas) {
				t.Errorf("expected %q in output, got %q", tc.wantHas, got)
			}
			if n != tc.wantCount {
				t.Errorf("redaction count = %d, want %d (out: %q)", n, tc.wantCount, got)
			}
		})
	}
}

func TestRedact_CapsLongExcerpt(t *testing.T) {
	long := strings.Repeat("a", 400) + "ERROR: connection refused" + strings.Repeat("b", 400)
	got, _ := Redact(long)
	if len(got) > excerptCap+len(elision) {
		t.Errorf("excerpt not capped: len=%d", len(got))
	}
	// The tail (where the error usually lands) should survive the elision.
	if !strings.Contains(got, "bbb") {
		t.Errorf("tail dropped entirely: %q", got)
	}
}
