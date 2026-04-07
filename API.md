# API reference

## Commands

```bash
looptap run                          # parse transcripts, detect signals
looptap info                         # what's in the db?
looptap parse                        # just parse, no signals
looptap signal --recompute           # re-run detectors on existing data
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

## Config

Optional. `~/.looptap/config.toml`:

```toml
[sources]
paths = ["~/extra-logs/"]     # additional transcript directories

[signals]
stagnation_similarity = 0.8   # how similar is "repeating yourself"
loop_window = 6               # sliding window for tool call loops
```

## Datasette canned queries

Ships with: signal summary, hotspots, failure log, loop log, signals by project, vibes check, model comparison, exhaustion events.

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```
