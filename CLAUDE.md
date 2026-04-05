# looptap

`strace` but for vibes. Reads coding agent transcripts, spots behavioral patterns, dumps it all into SQLite for datasette.

## Build

```bash
go build -o looptap . && ./looptap info --db /tmp/test.db
```

## Status

Full pipeline works — Claude Code parser, all 7 signal detectors, SQLite, datasette views, LLM advisor. Codex parser is stubbed. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture.

## Rules

- Standard library first. Dependencies earn their keep.
- No ORMs. Raw SQL only.
- Table-driven tests.
- `cmd/` stays boring — flag parsing and one function call.
- `internal/` is a library — no `os.Exit`, no global state.

## Tone

Have fun with it. Commits, comments, error messages — if it reads like a textbook, rewrite it. Check `git log` for the vibe.
