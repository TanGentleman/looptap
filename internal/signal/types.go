package signal

// Signal represents a detected behavioral signal in a session.
type Signal struct {
	SessionID  string
	Type       string  // "misalignment", "stagnation", "disengagement", "satisfaction", "failure", "loop", "exhaustion"
	Category   string  // "interaction", "execution", "environment"
	TurnIdx    *int    // nil = session-level
	Confidence float64 // 0.0–1.0
	Evidence   string  // phrase or pattern that matched
}
