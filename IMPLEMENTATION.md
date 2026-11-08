# looptap: implementation

## Requirements

- Go 1.26 or newer; CI uses 1.27.3.
- A C compiler: the SQLite driver, `github.com/mattn/go-sqlite3`, uses cgo. CI
  installs `libsqlite3-dev`.
- Only local files: looptap reads transcripts and writes the SQLite database. It
  makes no network requests.

## Usage

| Command | Does |
| --- | --- |
| `run [dir...]` | `parse`, then `signal` |
| `parse [dir...]` | find transcripts and store them; files already stored (same SHA-256) are skipped |
| `signal [--recompute]` | run the detectors over sessions not yet scored; `--recompute` redoes all |
| `query` | sessions with matching signals, as `jsonl`, `paths` or `tsv` |
| `patterns` | group recurring failures by tool and error class; `--format json` prints a `tracers.rule/v1` bundle |
| `info` | session, turn and signal counts |
| `seed-contract-fixture` | write the fixed two-cluster dataset that `patterns` tests use |
| `version` | build version |

Every command takes `--db PATH` (default `~/.looptap/looptap.db`); `--help`
lists the rest of the flags. Exit code is 0 on success, including an empty
`query` result, and 1 on any error.

On the bundled fixtures, after `run`:

```sh
./looptap query --db /tmp/demo.db --signal failure --format paths
# testdata/codex/rollout-2026-11-01T10-00-00-0199a000-0000-7000-8000-000000000001.jsonl
./looptap info --db /tmp/demo.db
```

On your own transcripts, run `looptap run` with no arguments; it writes to
`~/.looptap/looptap.db`.

`patterns` needs at least `--min-sessions` (default 5) sessions with the same
failure before it prints a rule. To see output without a transcript history:

```sh
./looptap seed-contract-fixture --db /tmp/p.db
./looptap patterns --db /tmp/p.db
```

Sources:

| Source | Default | Override |
| --- | --- | --- |
| Claude Code | `~/.claude/projects` | `CLAUDE_CONFIG_DIR` (reads `$CLAUDE_CONFIG_DIR/projects`) |
| Codex | `~/.codex/sessions`, `~/.codex/archived_sessions` | `CODEX_HOME` |

Directories passed to `run` or `parse` replace the defaults.

`~/.looptap/config.toml` is optional. Only `[database] path` and
`[sources] paths` take effect; the `[signals]` and `[phrases]` keys described in
ARCHITECTURE.md are parsed and ignored.

Browse the database with [datasette](https://datasette.io/):
`uvx datasette ~/.looptap/looptap.db --metadata metadata.json`.

Install from a clone with `go install .` (puts `looptap` in `$(go env GOPATH)/bin`).
`scripts/install.sh` downloads a GitHub release binary and checks it against
`SHA256SUMS`.

## How it works

Parsers (`internal/parser/`, one per agent format) turn transcript files into
sessions and turns. The detectors (`internal/signal/`) are pure functions over the
turns:

| Signal | Fires on |
| --- | --- |
| `misalignment` | correction phrases in user turns, or a user turn that repeats the previous one |
| `stagnation` | consecutive assistant turns that say the same thing |
| `disengagement` | give-up phrases, or a short final user turn that isn't thanks |
| `satisfaction` | thanks or success phrases in the last three user turns |
| `failure` | tool results marked as errors, or error text (traceback, exit code, build failed) |
| `loop` | the same tool called three or more times with similar arguments |
| `exhaustion` | rate-limit, context-length and timeout text |

Phrase lists are in `internal/signal/phrases/`. `patterns` clusters failure
signals across sessions (`internal/patterns`) and writes one CLAUDE.md/AGENTS.md
rule per cluster (`internal/rule`). Thresholds and the schema are in
[ARCHITECTURE.md](ARCHITECTURE.md). `patterns --format json` and
`docs/contracts.md` describe the hand-off to tracers, a separate project; looptap
does not send anything.

## Tests

CI (`.github/workflows/ci.yml`) on Go 1.27.3. Lint job:

```sh
sudo apt-get update
sudo apt-get install -y shellcheck
go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
go install mvdan.cc/sh/v3/cmd/shfmt@v3.14.1
actionlint
shellcheck scripts/*.sh
shfmt -d scripts/*.sh
./scripts/ci-guardrails.sh
```

Build and test job (`-race` needs cgo):

```sh
sudo apt-get update && sudo apt-get install -y libsqlite3-dev
go vet ./...
go test -race -count=1 ./...
go build -o looptap .
./scripts/smoke-db.sh ./looptap /tmp/ci-smoke.db
./looptap run testdata/claude_code testdata/codex --db /tmp/demo.db
./looptap query --db /tmp/demo.db --signal failure --format paths | grep -qx 'testdata/codex/rollout-2026-11-01T10-00-00-0199a000-0000-7000-8000-000000000001.jsonl'
./looptap info --db /tmp/demo.db
./looptap seed-contract-fixture --db /tmp/p.db
./looptap patterns --db /tmp/p.db | grep -q 'ENOENT'
```

Unit tests sit next to each package; `main_test.go` runs the fixture example end to
end, twice, and checks `query` per signal.

## Limits

- Compressed Codex rollouts (`.jsonl.zst`) are skipped.
- The Codex fixture is written from the Codex Rust types, not captured from a
  real session.
- The newest release, v0.7.0, predates `patterns`, `seed-contract-fixture` and
  the Codex parser, so `scripts/install.sh` installs a binary without them.
  `go install github.com/TanGentleman/looptap@latest` fails until a release with
  the `github.com/TanGentleman/looptap` module path is tagged.
- The release workflows (`release.yml`, `release-smoke.yml`, `cut-release.yml`)
  are linted but have not run since these changes.
