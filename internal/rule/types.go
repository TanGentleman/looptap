// Package rule is the record looptap and tracers agree on.
//
// One shape ties together the three things looptap learns from a pile of
// transcripts: the failure *pattern* it sees recurring across sessions, the
// *evidence* turns that prove it, and the *rule* worth pasting into an
// AGENTS.md / CLAUDE.md to stop it happening again. It is the unification of
// what used to be two ships passing in the night — advise.Recommendation and
// analyze.Finding — and the wire format tracers parses at its share boundary.
//
// The contract is tracers.rule/v1, specced in the tracers repo at
// docs/rule-with-evidence.md. JSON tags here are load-bearing: they ARE the
// contract. A round-trip test (types_test.go) pins them to the spec's golden
// example so a careless rename fails at `go test`, not in a downstream parser.
package rule

import (
	"bytes"
	"encoding/json"
	"time"
)

// Schema is the contract version string stamped into every Bundle.
// Bump the major (tracers.rule/v2) only for a breaking change — consumers
// ignore unknown fields, so additive growth stays on v1.
const Schema = "tracers.rule/v1"

// Bundle is the envelope: a versioned, timestamped batch of cards.
// An empty Cards slice is a valid bundle — "nothing crossed the gate" is an
// answer, not an error.
type Bundle struct {
	Schema          string `json:"schema"`
	GeneratedAt     string `json:"generated_at"`
	GateMinSessions int    `json:"gate_min_sessions"`
	Cards           []Card `json:"cards"`
}

// Card is one pattern, its evidence, and the rule it argues for.
type Card struct {
	ID        string     `json:"id"`
	Pattern   Pattern    `json:"pattern"`
	Evidence  []Evidence `json:"evidence"`
	Rule      Rule       `json:"rule"`
	Signature string     `json:"signature"` // tracers signs at the share boundary; "" until then
}

// Pattern is a failure shape clustered across sessions.
type Pattern struct {
	Signal            string   `json:"signal"`
	Tool              string   `json:"tool"`
	ErrorClass        string   `json:"error_class"`
	Summary           string   `json:"summary"`
	SessionCount      int      `json:"session_count"`
	ExampleSessionIDs []string `json:"example_session_ids"`
}

// Evidence is one redacted example turn — enough for a human to judge the
// pattern without trusting our verdict, not enough to leak a secret.
type Evidence struct {
	SessionID  string `json:"session_id"`
	TurnIdx    int    `json:"turn_idx"`
	ToolName   string `json:"tool_name"`
	IsError    bool   `json:"is_error"`
	Excerpt    string `json:"excerpt"`
	Redactions int    `json:"redactions"` // how many secrets the pre-pass scrubbed
}

// Rule is the ready-to-paste remediation.
type Rule struct {
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`    // the line(s) you actually paste
	Rationale  string `json:"rationale"`  // why, in one breath
	Target     string `json:"target"`     // "AGENTS.md" or "CLAUDE.md"
	Confidence string `json:"confidence"` // "high" | "medium" | "low"
	Source     string `json:"source"`     // "template" (deterministic) | "llm" (polished)
}

// Rule.Target values.
const (
	TargetAgents = "AGENTS.md"
	TargetClaude = "CLAUDE.md"
)

// Rule.Source values.
const (
	SourceTemplate = "template"
	SourceLLM      = "llm"
)

// NewBundle wraps cards in the envelope, stamping the current schema and a
// UTC RFC3339 generation time. A nil cards slice becomes an empty (not null)
// JSON array so consumers never have to special-case it.
func NewBundle(cards []Card, gateMinSessions int) Bundle {
	if cards == nil {
		cards = []Card{}
	}
	return Bundle{
		Schema:          Schema,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		GateMinSessions: gateMinSessions,
		Cards:           cards,
	}
}

// MarshalBundle is the one-call path from cards to bytes-on-the-wire: build the
// envelope and indent it for the humans who'll inevitably read it in a pipe.
// HTML escaping is off so snippets keep their literal `<dir>` rather than
// turning into <dir> — it's still valid JSON for tracers to parse.
// No trailing newline; the caller decides how to terminate.
func MarshalBundle(cards []Card, gateMinSessions int) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(NewBundle(cards, gateMinSessions)); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
