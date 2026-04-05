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
```

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
go build -o looptap .    # needs CGo for SQLite
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
pip install datasette datasette-vega
datasette ~/.looptap/looptap.db
```

## Status

- [x] CLI with all commands
- [x] Claude Code transcript parser
- [ ] Codex transcript parser
- [ ] Signal detection logic (stubs in place)
- [ ] Datasette canned queries

See [ARCHITECTURE.md](ARCHITECTURE.md) for the technical deep dive.
