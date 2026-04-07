# looptap

`strace` but for vibes. Reads coding agent transcripts, spots behavioral patterns, dumps it all into SQLite for datasette.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/install.sh | bash
```

Or build from source (needs CGo for SQLite):

```bash
go build -o looptap .
```

## Use

```bash
looptap run           # parse transcripts + detect signals
looptap info          # what's in the db?
looptap advise        # ask Gemini for CLAUDE.md suggestions (needs GOOGLE_API_KEY)
```

All commands take `--db <path>` (default `~/.looptap/looptap.db`).

## Browse

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

## Go deeper

- [API.md](API.md) — all commands, signals, config options, datasette queries
- [ARCHITECTURE.md](ARCHITECTURE.md) — how the pipeline works, schema, internals
- [CLAUDE.md](CLAUDE.md) — dev rules and conventions
