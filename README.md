# looptap

looptap parses local coding agent transcripts (Claude Code, Codex, etc.), computes lightweight behavioral signals, and writes everything to a SQLite database. You point [datasette](https://datasette.io/) at the DB for visualization.

The name: you're tapping into the feedback loop between developer and agent.

## What it does

```
local transcript files → parse → SQLite → signal → SQLite (enriched)
                                                         ↓
                                                    datasette / any SQL client
```

### Commands

| Command | Purpose |
|---------|---------|
| `looptap parse` | Discover agent transcripts, normalize to common schema, write to SQLite. Incremental (skips unchanged files by hash). |
| `looptap signal` | Run signal detectors over parsed sessions, write results to `signals` table. Skips already-signaled sessions unless `--recompute`. |
| `looptap run` | `parse` then `signal`. |
| `looptap info` | Print DB stats (session/turn/signal counts by source and type). |

Global flag: `--db <path>` (default `~/.looptap/looptap.db`).

## Building

```bash
go build -o looptap .
```

Requires CGo for SQLite (`github.com/mattn/go-sqlite3`).

## Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/mattn/go-sqlite3` | SQLite driver (CGo) |
| `github.com/BurntSushi/toml` | Config parsing |
| `github.com/stretchr/testify` | Test assertions |

No web framework. No ORM. No LLM client libraries.

## Project layout

```
looptap/
├── main.go                        # cobra root command, calls cmd/
├── cmd/
│   ├── parse.go                   # parse command wiring
│   ├── signal.go                  # signal command wiring
│   ├── run.go                     # run command wiring
│   └── info.go                    # info command wiring
├── internal/
│   ├── config/config.go           # load ~/.looptap/config.toml
│   ├── db/
│   │   ├── db.go                  # Open(), Migrate(), Close()
│   │   └── queries.go             # InsertSession(), GetSession(), InsertSignals(), etc.
│   ├── parser/
│   │   ├── types.go               # Session, Turn structs
│   │   ├── parser.go              # Parser interface, Detect() auto-selection, Discover()
│   │   ├── claude_code.go         # Claude Code JSONL parser
│   │   └── codex.go               # Codex CLI parser
│   └── signal/
│       ├── types.go               # Signal struct
│       ├── detector.go            # Detector interface, RunAll()
│       ├── text.go                # Normalize(), TokenSimilarity(), MatchPhrases()
│       ├── misalignment.go        # correction & rephrasing detection
│       ├── stagnation.go          # repetitive assistant behavior
│       ├── disengagement.go       # user abandonment patterns
│       ├── satisfaction.go        # positive feedback patterns
│       ├── failure.go             # tool execution errors
│       ├── loop.go                # repeated tool call patterns
│       └── exhaustion.go          # rate limits, context length, timeouts
├── phrases/                       # go:embed, one phrase per line
│   ├── misalignment.txt
│   ├── disengagement.txt
│   └── satisfaction.txt
└── testdata/                      # fixture transcripts (coming soon)
```

All domain logic lives in `internal/`. The `cmd/` layer is glue — it parses flags, calls `internal/`, and prints output. `internal/` packages are importable as a Go library.

## Configuration

`~/.looptap/config.toml`:

```toml
[database]
path = "~/.looptap/looptap.db"

[sources]
paths = ["~/extra-logs/"]

[signals]
stagnation_similarity  = 0.8
stagnation_turn_factor = 2.0
loop_window            = 6
loop_min_repeats       = 3

[phrases]
# misalignment = "/path/to/replace.txt"
# misalignment_extra = "/path/to/append.txt"
```

## Signal types

| Signal | Category | What it catches |
|--------|----------|----------------|
| Misalignment | interaction | User corrections, rephrasing the same request |
| Stagnation | interaction | Assistant repeating itself, unusually long sessions |
| Disengagement | interaction | User giving up, terse final messages |
| Satisfaction | interaction | Gratitude, success phrases |
| Failure | execution | Tool errors, stack traces, nonzero exit codes |
| Loop | execution | Same tool called repeatedly with similar input |
| Exhaustion | environment | Rate limits, context length, timeouts |

## Datasette

```bash
pip install datasette datasette-vega
datasette ~/.looptap/looptap.db
```

## Status

Scaffold is in place and the binary compiles. Next up:
- [ ] Implement Claude Code JSONL parser
- [ ] Implement Codex parser
- [ ] Wire up signal detection logic
- [ ] Add test fixtures and golden file tests
- [ ] Ship `datasette/metadata.yml` with canned queries
