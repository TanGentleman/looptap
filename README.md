# looptap

You talk to coding agents. They talk back. But what's *actually* happening in those conversations?

looptap reads your local agent transcripts, looks for behavioral patterns — correction loops, stagnation, quiet satisfaction, tool call death spirals — and writes it all to SQLite. Point [datasette](https://datasette.io/) at the DB and suddenly you can *see* the feedback loop.

```
transcripts → parse → SQLite → signal → SQLite (enriched)
                                              ↓
                                         datasette / SQL
```

## Usage

```bash
looptap run                          # parse transcripts, detect signals
looptap info                         # what's in the db?
looptap parse                        # just parse, no signals
looptap signal --recompute           # re-run detectors on everything
looptap advise                       # ask Gemini for CLAUDE.md suggestions
looptap advise --project myapp       # scope to one project
looptap advise --json                # machine-readable output
```

`advise` requires a Gemini API key: `GOOGLE_API_KEY` env var, `--api-key` flag, or `[advise] api_key` in config.

All commands take `--db <path>` (default `~/.looptap/looptap.db`).

## Signals

| Signal | Vibes |
|--------|-------|
| **Misalignment** | "no, that's not what I meant" — user correcting / rephrasing |
| **Stagnation** | assistant saying the same thing in different fonts |
| **Disengagement** | "nvm I'll do it myself" |
| **Satisfaction** | "perfect, thanks!" (the good ending) |
| **Failure** | stack traces, exit code 1, the red text |
| **Loop** | same tool, same args, same hope, different minute |
| **Exhaustion** | rate limits, context window exceeded, timeout |

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/install.sh | bash
```

Or build from source (needs CGo for SQLite):

```bash
go build -o looptap .
```

## Configure

Optional. `~/.looptap/config.toml`:

```toml
[sources]
paths = ["~/extra-logs/"]     # additional transcript directories

[signals]
stagnation_similarity = 0.8   # how similar is "repeating yourself"
loop_window = 6               # sliding window for tool call loops
```

## Browse

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

Ships with canned queries: signal summary, hotspots, failure log, loop log, signals by project, vibes check, model comparison, exhaustion events.

## Status

- [x] CLI with all commands
- [x] Claude Code transcript parser
- [x] Signal detection (all 7 detectors)
- [x] Datasette canned queries
- [x] LLM-powered CLAUDE.md advisor (`advise` command)
- [ ] Codex transcript parser

See [ARCHITECTURE.md](ARCHITECTURE.md) for the technical deep dive.
