package main

import "testing"

func TestRootCommands(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"parse", true},
		{"signal", true},
		{"run", true},
		{"info", true},
		{"query", true},
		{"patterns", true},
		{"seed-contract-fixture", true},
		{"version", true},
	}
	root := newRootCmd()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{tt.name})
			got := err == nil && cmd != root && cmd.Name() == tt.name
			if got != tt.want {
				t.Errorf("command %q registered = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
