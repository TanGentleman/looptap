package signal

import "testing"

func TestErrorClass(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Known tokens — errno and its English message collapse together.
		{"enoent message", "bash: cd: packages/api: No such file or directory", "ENOENT"},
		{"enoent errno", "Error: ENOENT: no such file", "ENOENT"},
		{"permission", "open /etc/shadow: permission denied", "EACCES"},
		{"conn refused", "dial tcp 127.0.0.1:5432: connection refused", "ECONNREFUSED"},
		{"module before command", "Error: Cannot find module 'express'", "module-not-found"},
		{"command not found", "bash: foobar: command not found", "command-not-found"},
		{"exit code", "npm test exited with exit code 1", "exit-code"},
		{"timeout", "context deadline exceeded", "timeout"},
		{"rate limit", "429 Too Many Requests", "rate-limit"},
		{"oom", "fatal: out of memory", "oom"},
		{"python traceback", "Traceback (most recent call last):\n  File ...", "python-traceback"},

		// Generic fallback scrubs the volatile bits so siblings cluster together.
		{"empty", "", ""},
		{"whitespace only", "   \n\t ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ErrorClass(tc.in); got != tc.want {
				t.Errorf("ErrorClass(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Two structurally-identical unknown errors that differ only in path/number
// must land in the same generic family — that's the whole point of scrubbing.
func TestErrorClass_GenericFamiliesAgree(t *testing.T) {
	a := ErrorClass("weird gizmo failed at /Users/ann/api/foo.ts line 23")
	b := ErrorClass("weird gizmo failed at /home/bob/api/bar.ts line 99")
	if a == "" || a != b {
		t.Errorf("siblings disagree:\n a = %q\n b = %q", a, b)
	}
}

// Distinct errors must NOT collapse into one bucket.
func TestErrorClass_DistinctStaysDistinct(t *testing.T) {
	if ErrorClass("connection refused") == ErrorClass("no such file or directory") {
		t.Error("ECONNREFUSED and ENOENT collapsed into one class")
	}
}
