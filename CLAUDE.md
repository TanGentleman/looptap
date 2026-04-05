# CLAUDE.md — looptap

## What is this?

looptap is a Go CLI that parses coding agent transcripts (Claude Code, Codex, etc.), runs behavioral signal detectors over them, and dumps everything into SQLite. Datasette handles the UI. Think of it as `strace` but for vibes.

## Quick orientation

```
main.go          → cobra root, wires cmd/
cmd/             → thin CLI glue (parse, signal, run, info)
internal/parser/ → transcript ingestion (Parser interface)
internal/signal/ → behavioral detectors (Detector interface)
internal/db/     → SQLite schema + queries
internal/config/ → ~/.looptap/config.toml loader
phrases/         → embedded phrase lists for signal matching
```

All the interesting logic lives in `internal/`. The `cmd/` layer should stay boring.

## Build & run

```bash
go build -o looptap .    # needs CGo for SQLite
./looptap info --db /tmp/test.db   # quick smoke test
```

## Architecture — two interfaces, that's it

**Parser** (`internal/parser/parser.go`): one implementation per agent format. Implement `Name()`, `CanParse(path)`, `Parse(path) (Session, error)`. Register in the `parsers` slice. Current stubs: `ClaudeCode`, `Codex`.

**Detector** (`internal/signal/detector.go`): one implementation per signal type. Implement `Name()`, `Category()`, `Detect(Session) []Signal`. Register in the `All` slice. Seven stubs waiting to be filled in.

Adding a new agent or signal = one file implementing one interface. No plugin system, no reflection, no drama.

## Current state (what's done vs TODO)

**Done:**
- Full project skeleton, compiles clean
- CLI with all 4 commands working
- SQLite schema with auto-migration
- DB query layer (insert/get sessions, signals, stats)
- Config loading with sensible defaults
- Parser discovery + file walking (real)
- `CanParse` for Claude Code and Codex (real path matching)
- Text utilities: `Normalize`, `TokenSimilarity`, `MatchPhrases` with Levenshtein edit distance
- Phrase files embedded

**TODO:**
- `Parse()` for Claude Code (`internal/parser/claude_code.go`) — needs JSONL schema work
- `Parse()` for Codex (`internal/parser/codex.go`) — needs transcript format confirmation
- `Detect()` for all 7 signal detectors — currently return nil
- Test fixtures in `testdata/`
- Golden file snapshot tests
- `datasette/metadata.yml`

## Conventions

- **Standard library first.** Only add a dependency if it earns its keep.
- **No ORMs.** Raw SQL in `internal/db/queries.go`. The schema is in `internal/db/db.go` as migration strings.
- **Table-driven tests.** See the README for the pattern.
- **Comments should spark joy.** If you're writing a comment, make it worth reading. Dry wit > dry documentation. A comment that makes someone smile is a comment that gets read. No comment is better than a comment that just restates the code — but a comment that tells the *story* of why something is weird? Chef's kiss.

## Common tasks

### Adding a new parser
1. Create `internal/parser/my_agent.go`
2. Implement the `Parser` interface
3. Add `&MyAgent{}` to the `parsers` slice in `parser.go`
4. Add test fixtures in `testdata/my_agent/`

### Adding a new signal detector
1. Create `internal/signal/my_signal.go`
2. Implement the `Detector` interface
3. Add `&MySignal{}` to the `All` slice in `detector.go`
4. Phrase lists go in `phrases/my_signal.txt` if needed

### Implementing an existing stub detector
The detectors in `internal/signal/` all have TODOs describing the detection rules. The README has full specs. `signal/text.go` already has the shared utilities you'll need (`Normalize`, `TokenSimilarity`, `MatchPhrases`).

## Style notes

- Keep `cmd/` files thin — flag parsing and one function call, that's the dream
- `internal/` packages should be usable as a Go library (no `os.Exit`, no global state beyond the detector registry)
- Error messages should help the human debug, not just say "something went wrong"
- When in doubt, look at how the existing code does it and match that energy
