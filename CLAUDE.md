# CLAUDE.md — looptap

## What is this?

`strace` but for vibes. A Go CLI that reads coding agent transcripts, spots behavioral patterns, and dumps everything into SQLite for poking at with datasette.

## Orientation

```
main.go          → cobra root, wires cmd/
cmd/             → thin CLI glue (should stay boring)
internal/parser/ → transcript ingestion (Parser interface)
internal/signal/ → behavioral detectors (Detector interface)
internal/db/     → SQLite schema + queries (raw SQL, no ORM)
internal/config/ → ~/.looptap/config.toml
phrases/         → embedded phrase lists for signal matching
```

## Build & smoke test

```bash
go build -o looptap .
./looptap info --db /tmp/test.db
```

## Current state

**Working:** CLI, SQLite schema, config, Claude Code parser (full JSONL parsing), text utilities, parser discovery. **Stub:** Codex parser (CanParse works, Parse doesn't), all 7 signal detectors (return nil).

See [ARCHITECTURE.md](ARCHITECTURE.md) for interfaces, schemas, and detection rules.

## How to add things

**New parser:** one file in `internal/parser/`, implement `Parser` interface, register in `parsers` slice.
**New signal:** one file in `internal/signal/`, implement `Detector` interface, register in `All` slice.
**Phrase list:** add `phrases/my_signal.txt`, one phrase per line.

## Conventions

- **Standard library first.** Dependencies earn their keep or they leave.
- **No ORMs.** SQL is a feature, not a problem.
- **Table-driven tests.**
- **`cmd/` stays thin.** Flag parsing and one function call — that's the dream.
- **`internal/` is a library.** No `os.Exit`, no global state beyond registries.
- **Error messages help humans debug.** "something went wrong" is never acceptable.

## Tone

Comments and commits should have personality. Dry wit over dry documentation. A comment that makes someone smile is a comment that gets read. If you're explaining *why* something is weird, that's worth writing. If you're restating what the code already says, delete it.

Commit titles: short, playful, evocative. Body can have details. See git log for the vibe.
