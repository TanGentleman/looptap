package procenv

import "testing"

func TestDebugOn(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"unset", "", false},
		{"zero", "0", false},
		{"true", "true", false},
		{"yes", "yes", false},
		{"padded", "1 ", false},
		{"one", "1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DebugOn(tc.in); got != tc.want {
				t.Errorf("DebugOn(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"blank slice", []string{}, ""},
		{"already sorted", []string{"A=1", "B=2"}, "A=1\nB=2"},
		{"sorts", []string{"ZED=9", "A=1", "M=mid"}, "A=1\nM=mid\nZED=9"},
		{"keeps equals in values", []string{"B=2", "A=x=y"}, "A=x=y\nB=2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Format(tc.in); got != tc.want {
				t.Errorf("Format(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormat_DoesNotReorderCaller(t *testing.T) {
	in := []string{"B=2", "A=1"}
	if got := Format(in); got != "A=1\nB=2" {
		t.Fatalf("Format = %q", got)
	}
	if in[0] != "B=2" || in[1] != "A=1" {
		t.Fatalf("Format reordered the caller slice: %q", in)
	}
}
