package patterns

import (
	"fmt"
	"time"

	"looptap/internal/db"
	"looptap/internal/parser"
	"looptap/internal/signal"
)

// The two canonical failure shapes the contract fixture plants. The ENOENT
// cluster clears the default gate of 5 (so it becomes a card); the
// connection-refused cluster sits below it (so it proves the gate bites). These
// strings carry tokens signal.ErrorClass recognises — "no such file or
// directory" -> ENOENT, "connection refused" -> ECONNREFUSED — so the engine
// clusters them without us hand-labelling anything.
const (
	enoentOutput = "bash: cd: packages/api: No such file or directory"
	connOutput   = "dial tcp 127.0.0.1:5432: connection refused"
)

// leakyOutput is an extra erroring turn we hang on the newest ENOENT session
// when --leaky is set: a transcript line that "accidentally" prints an
// obviously-fake Anthropic-shaped API key next to the same ENOENT error, so the
// turn still clusters into the ENOENT shape. It is the entire point of the live
// capture — it drags a redactable secret through real engine output.
//
// The key is BARE (no NAME= wrapper) on purpose. looptap's own pre-pass
// (internal/rule/redact.go) scrubs the `sk-...` token to [REDACTED] on the way
// out, so the emitted evidence never carries the raw secret — and because the
// scrub leaves a bare placeholder rather than `KEY=[REDACTED]`, the downstream
// authoritative redactor (tracers) sees nothing left to strip and agrees the
// excerpt is clean. A `NAME=value` shape would survive that second look and the
// "still contains a secret?" check would never settle to false.
const leakyOutput = "$ printenv ANTHROPIC_API_KEY   # leaked into the CI log\n" +
	"sk-ant-api03-NOTREAL00FAKE00FIXTURE00SEED00DONOTUSE\n" +
	enoentOutput

// SeedContractFixture plants the deterministic two-cluster fixture that
// internal/patterns' own tests assert against — 6 ENOENT sessions (above the
// gate) and 2 connection-refused sessions (below it), each carrying one erroring
// Bash turn the failure signal points at.
//
// It is the single source of truth for that shape: the patterns engine test
// seeds through it, and so does `looptap seed-contract-fixture`, so a consumer
// capturing live engine output and the library's own table tests can never drift
// apart on the session counts.
//
// When leaky is true the newest ENOENT session grows ONE extra erroring turn
// (and its own failure signal) whose output leaks a fake API key. That adds an
// evidence row to the ENOENT card without adding a distinct session — the
// cluster still counts 6 — so the only thing that moves is the evidence, which
// now exercises the redactor end to end.
func SeedContractFixture(d *db.DB, leaky bool) error {
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// errTurn is the standard user -> tool_use -> erroring tool_result triple a
	// failure signal at turnIdx 2 points at.
	errTurns := func(tool, content string) []parser.Turn {
		return []parser.Turn{
			{Idx: 0, Role: "user", Content: "do it"},
			{Idx: 1, Role: "tool_use", ToolName: tool, Content: "{}"},
			{Idx: 2, Role: "tool_result", ToolName: tool, IsError: true, Content: content},
		}
	}

	plant := func(id string, when time.Time, turns []parser.Turn, sigTurns ...int) error {
		s := parser.Session{
			ID: id, Source: "claude-code", Project: "/repo/app",
			SessionID: id, StartedAt: when, EndedAt: when.Add(time.Hour),
			RawPath: "/tmp/" + id + ".jsonl", FileHash: "h-" + id,
			Turns: turns,
		}
		if err := d.InsertSession(s); err != nil {
			return fmt.Errorf("insert %s: %w", id, err)
		}
		sigs := make([]signal.Signal, 0, len(sigTurns))
		for _, ti := range sigTurns {
			idx := ti
			sigs = append(sigs, signal.Signal{
				SessionID: id, Type: "failure", Category: "execution",
				Confidence: 0.9, Evidence: "exit 1", TurnIdx: &idx,
			})
		}
		if err := d.InsertSignals(id, sigs); err != nil {
			return fmt.Errorf("signals %s: %w", id, err)
		}
		return nil
	}

	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("enoent-%d", i)
		when := base.Add(time.Duration(i) * time.Hour)
		turns := errTurns("Bash", enoentOutput)
		sigTurns := []int{2}

		// The newest ENOENT session (the last one, base+5h) is the one that
		// surfaces first in the card's evidence, so that's where the leaky turn
		// rides — guaranteeing it lands in the top-3 excerpts the card carries.
		if leaky && i == 5 {
			turns = append(turns,
				parser.Turn{Idx: 3, Role: "tool_use", ToolName: "Bash", Content: "{}"},
				parser.Turn{Idx: 4, Role: "tool_result", ToolName: "Bash", IsError: true, Content: leakyOutput},
			)
			sigTurns = append(sigTurns, 4)
		}

		if err := plant(id, when, turns, sigTurns...); err != nil {
			return err
		}
	}

	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("conn-%d", i)
		when := base.Add(time.Duration(10+i) * time.Hour)
		if err := plant(id, when, errTurns("Bash", connOutput), 2); err != nil {
			return err
		}
	}

	return nil
}
