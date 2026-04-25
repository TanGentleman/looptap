# CLAUDE.md

Notes for agents working in this repo. The full picture lives in [ARCHITECTURE.md](ARCHITECTURE.md).

## Smoke build

```bash
go build -o looptap . && ./looptap info --db /tmp/test.db
```

## Directives

- Stdlib first; new dependencies have to earn it.
- Raw SQL only — no ORM.
- Tests are table-driven.
- `cmd/` is wiring (flag parsing, one call into `internal/`); `internal/` is library-shaped — no `os.Exit`, no globals.
- Have fun with commits, comments, and error messages. `git log` sets the tone.
