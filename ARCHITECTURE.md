# Architecture

> **Cross-system contract:** looptap's wire shape (`tracers.rule/v1`) is documented in [docs/contracts.md](docs/contracts.md). JSON Schemas: [docs/schemas/](docs/schemas/). Fixtures: [testdata/contracts/](testdata/contracts/). tracers workflow rows are **insights**.

## Two interfaces, that's it

Everything interesting in looptap flows through two interfaces. Each new agent format or signal type is one file implementing one interface. No plugin system, no reflection, no service locators.

### Parser

```go
// internal/parser/parser.go
type Parser interface {
    Name() string
    CanParse(path string) bool
    Parse(path string) (Session, error)
}
```

Registered in a package-level slice. `Detect(path)` iterates and returns the first match. `Discover(dirs)` walks directories and collects everything parseable.

Implementations: `ClaudeCode`, `Codex`.

### Detector

```go
// internal/signal/detector.go
type Detector interface {
    Name() string
    Category() string
    Detect(s parser.Session) []Signal
}
```

Also a package-level slice (`All`). `RunAll(session)` fans out to every detector. No network calls, no LLM calls — pure functions over turns.

## The pipeline: signal → pattern → rule

Everything downstream of parsing is one three-rung ladder, each rung a wider lens on the same transcripts:

| Rung | Scope | Determinism | Lives in |
|------|-------|-------------|----------|
| **Signal** | one turn / one session | pure | `internal/signal` |
| **Pattern** | a failure shape recurring *across* sessions | pure | `internal/patterns` |
| **Rule** | the remediation, with evidence | pure (templates) | `internal/rule` |

`run`/`signal` populate the bottom rung. `patterns` clusters signals into the middle rung. `rule` synthesizes the top rung — a ready-to-paste AGENTS.md / CLAUDE.md line — deterministically from a cluster. The top rung is also the wire format looptap shares with tracers (a private repo): `tracers.rule/v1`.

## Core types

```go
// internal/parser/types.go
type Session struct {
    ID        string    // sha256(Source + SessionID)
    Source    string    // "claude-code", "codex"
    Project   string
    SessionID string    // original ID from agent
    StartedAt time.Time
    EndedAt   time.Time
    Model     string
    GitBranch string
    RawPath   string
    FileHash  string    // SHA-256 of file contents
    Turns     []Turn
}

type Turn struct {
    Idx      int
    Role     string    // "user", "assistant", "tool_use", "tool_result", "system"
    Content  string
    Time     time.Time
    ToolName string
    IsError  bool
}

// internal/signal/types.go
type Signal struct {
    SessionID  string
    Type       string   // "misalignment", "stagnation", etc.
    Category   string   // "interaction", "execution", "environment"
    TurnIdx    *int     // nil = session-level
    Confidence float64  // 0.0–1.0
    Evidence   string   // phrase or pattern that matched
}
```

## SQLite schema

Auto-migrated on `db.Open()`. Lives in `internal/db/db.go` as versioned SQL strings.

```sql
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    source      TEXT NOT NULL,
    project     TEXT,
    session_id  TEXT NOT NULL,
    started_at  TEXT,
    ended_at    TEXT,
    model       TEXT,
    total_turns INTEGER NOT NULL,
    tool_calls  INTEGER NOT NULL DEFAULT 0,
    git_branch  TEXT,
    raw_path    TEXT NOT NULL,
    file_hash   TEXT NOT NULL,
    parsed_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    signaled_at TEXT
);

CREATE TABLE turns (
    session_id TEXT NOT NULL REFERENCES sessions(id),
    idx        INTEGER NOT NULL,
    role       TEXT NOT NULL,
    content    TEXT,
    timestamp  TEXT,
    tool_name  TEXT,
    is_error   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, idx)
);

CREATE TABLE signals (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id      TEXT NOT NULL REFERENCES sessions(id),
    signal_type     TEXT NOT NULL,
    signal_category TEXT NOT NULL,
    turn_idx        INTEGER,
    confidence      REAL NOT NULL,
    evidence        TEXT
);
```

Indexes on `signals(signal_type)`, `signals(session_id)`, `sessions(source)`, `sessions(project)`.

## Claude Code JSONL format

Each line is a JSON object with a `type` field. We parse `"user"` and `"assistant"`, skip everything else (`file-history-snapshot`, `progress`, `last-prompt`, `summary`).

**User messages:** `message.content` is either a string (plain text) or an array of content blocks (tool results). Lines with `isMeta` (injected caveats, skill bodies, hook output) or `isCompactSummary` (the summary written after `/compact`) are skipped: the user didn't type them.

**Assistant messages:** `message.content` is always an array — `text`, `tool_use`, and `thinking` blocks. Thinking blocks are skipped.

**Turn mapping:**
- User string content → `Role: "user"`
- User tool_result block → `Role: "tool_result"`, `IsError` from `is_error`
- Assistant text blocks → accumulated into one `Role: "assistant"` turn
- Assistant tool_use block → `Role: "tool_use"`, `ToolName` from `name`, `Content` is JSON of input

**Metadata:** `sessionId`, `cwd` (→ Project), `gitBranch`, `message.model`, timestamps from first/last lines. Session ID is `sha256("claude-code" + sessionId)`.

Subagent transcripts (in `<session>/subagents/`) are skipped. Files under `.claude/projects/` are taken on path alone; anything else (`$CLAUDE_CONFIG_DIR/projects`, a directory passed to `parse`) is accepted if one of its first 20 lines is a `user`/`assistant` record with a `sessionId`.

## Codex rollout format

Rollouts live in `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-*.jsonl` (and `archived_sessions/`). Each line is `{"timestamp", "type", "payload"}`.

**Turn mapping:**
- `event_msg` / `user_message` → `Role: "user"`. The matching `response_item` user message is skipped, as are the injected AGENTS.md, environment and developer messages.
- `response_item` assistant `message` → `Role: "assistant"`, `output_text` parts joined
- `function_call`, `custom_tool_call`, `local_shell_call` → `Role: "tool_use"`
- `function_call_output`, `custom_tool_call_output` → `Role: "tool_result"`, `ToolName` from the call, `IsError` when the output header (`Exit code: N` or `Process exited with code N`, wall time, ..., `Output:`) has a non-zero code; the header is dropped from `Content` so the failure detector's "exit code" pattern doesn't fire on every command. Older `{"output","metadata":{"exit_code"}}` outputs are unwrapped the same way.
- `reasoning`, `compacted`, token counts and other events are skipped

**Metadata:** first `session_meta` gives `id`, `cwd`, `git.branch`; `turn_context.model` gives the model. Session ID is `sha256("codex" + id)`. Compressed `.jsonl.zst` rollouts are not read.

## Signal detection rules

All detectors operate on `Session` + `[]Turn`. No network. No LLM.

| Signal | Category | Detection |
|--------|----------|-----------|
| **Misalignment** | interaction | Correction phrases from `internal/signal/phrases/misalignment.txt` (edit-distance tolerant). Consecutive user turns with token similarity > 0.7. |
| **Stagnation** | interaction | Consecutive assistant turns with token similarity > 0.8. |
| **Disengagement** | interaction | Final user turn ≤ 5 words and not a satisfaction phrase. Abandonment phrases from `internal/signal/phrases/disengagement.txt`. |
| **Satisfaction** | interaction | Gratitude/success phrases from `internal/signal/phrases/satisfaction.txt` in final 3 user turns. |
| **Failure** | execution | `tool_result` turns with `IsError == true`. Error patterns in content (stack traces, "command failed", "exit code"). |
| **Loop** | execution | Sliding window of 6 over `tool_use` turns. Flag when ≥ 3 share the same ToolName with content similarity > 0.8. |
| **Exhaustion** | environment | Rate-limit/context-length/timeout phrases from `internal/signal/phrases/exhaustion.txt` in `tool_result`, `system` and `assistant` turns. |

## Text utilities (`internal/signal/text.go`)

```go
func Normalize(s string) string           // lowercase, strip punctuation, collapse whitespace
func TokenSimilarity(a, b string) float64 // Jaccard similarity on whitespace-split tokens
func MatchPhrases(text string, phrases []string, maxEditDist int) (bool, string)
```

`MatchPhrases` does exact substring match first, then falls back to Levenshtein edit distance on word-level sliding windows.

## Query (`cmd/query.go` + `db.QuerySessions`)

The `query` command is the read side of the pipeline: "which transcripts hit signals X, Y, Z?". One SQL `JOIN sessions ↔ signals` filtered by a `db.QueryFilter`, with the matching signals attached to each session.

**Input** — flags: `--signal` (repeatable, OR-joined), `--min-confidence`, `--source`, `--project` (substring), `--since` / `--until` (`YYYY-MM-DD` or RFC3339), `--limit`.

**Output** — one record per session, sorted `started_at DESC`:
- `--format jsonl` (default): `{session_id, source, project, raw_path, file_hash, started_at, ended_at, signals:[…]}`. Only *matching* signals — caller asked for those.
- `--format paths`: `raw_path` only. `xargs tar` fodder.
- `--format tsv`: `session_id\traw_path\tsignal_count\ttypes`.

Empty match exits 0 — silent is a valid answer. Bad format strings exit 1.

## Patterns (`internal/patterns/` + `cmd/patterns.go`)

`query` finds *which sessions* hit a signal. `patterns` is the rung up: it finds *what keeps going wrong across all of them*, with zero LLM calls.

```
signals ⋈ turns ⋈ sessions → Find() (SQL + Go grouping) → []Cluster → rule.Card
```

The clustering key is **(signal_type, tool_name, error_class)**. The error class comes from `signal.ErrorClass` (in `text.go`, next to the other text utilities), which distills messy output down to a stable family: known tokens collapse errno and message together (`ENOENT` == "no such file or directory"), and anything unrecognized is scrubbed of paths/uuids/hex/digits so structural siblings still cluster. No schema change — `turns` already carries `tool_name`/`content`/`is_error`, `signals` carries `evidence`.

`Find(conn, Options)` returns `[]Cluster` sorted by distinct-session count descending (most pervasive first). Each `Cluster` projects onto a `rule.Pattern` and synthesizes a `rule.Card`. The **gate** (`--min-sessions`, default 5) is a publishing decision the *command* makes, not the engine — `Find` reports everything; `cmd/patterns.go` decides what becomes a card.

**Output** — `--format text` (default) lists every cluster human-first, marking the ones below the gate; `--format json` emits a `tracers.rule/v1` bundle of only the clusters that cleared the gate (or all of them with `--include-below-gate`, which writes an engine-debug warning to stderr — tracers skips sub-gate cards on ingest). The bundle declares `gate_min_sessions` from `--min-sessions` so the consumer cannot drift from the producer. Empty `cards[]` is a valid bundle.

## Rule record (`internal/rule/` — `tracers.rule/v1`)

The shared record: a **Pattern** (failure shape), **Evidence** (redacted example turns), and the **Rule** worth pasting somewhere. `internal/rule/types.go` is the source — the JSON tags *are* the contract, and `types_test.go` round-trips the golden so a careless rename fails at `go test`.

- **`synth.go`** — `Synthesize(pattern, examples)` builds a `Card` deterministically; `confidence` is set by session count (≥10 high, ≥5 medium, else low); `source = "template"`.
- **`redact.go`** — best-effort pre-pass: caps excerpts (~600 chars), scrubs obvious secrets. **Not authoritative** — tracers re-redacts at ingest and at share.
- **`seed.go` + `cmd/seed-contract-fixture`** — `patterns.SeedContractFixture(db, leaky)` plants the deterministic two-cluster fixture (6 ENOENT over gate, 2 connection-refused under). `--leaky` hangs an extra erroring turn carrying a fake API key on the newest ENOENT session so the capture exercises the redaction pre-pass against real engine output.

Wire shape, argv stability, redaction layering: [docs/contracts.md](docs/contracts.md). Schema: [docs/schemas/tracers.rule.v1.json](docs/schemas/tracers.rule.v1.json).

## Config (`~/.looptap/config.toml`)

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
misalignment = "/path/to/replace.txt"       # override built-in phrases
misalignment_extra = "/path/to/append.txt"  # add to built-in phrases
```

Only `[database] path` and `[sources] paths` take effect. `[signals]` and `[phrases]` are parsed but no detector reads them yet; the thresholds and phrase lists above are fixed in the code. Unknown sections are ignored, so a leftover `[advise]` from older versions still loads.

## Dependencies

| Dependency | Why |
|------------|-----|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/mattn/go-sqlite3` | SQLite driver (CGo) |
| `github.com/BurntSushi/toml` | Config parsing |
| `github.com/santhosh-tekuri/jsonschema/v6` | Validates contract fixtures in tests |

No web framework. No ORM.

## Project layout

```
main.go                        # cobra root
cmd/                           # CLI command wiring
internal/config/config.go      # config loading
internal/db/db.go              # Open(), Migrate(), Close()
internal/db/queries.go         # all SQL queries
internal/parser/types.go       # Session, Turn
internal/parser/parser.go      # Parser interface, Detect(), Discover()
internal/parser/claude_code.go # Claude Code JSONL parser
internal/parser/codex.go       # Codex parser
internal/signal/types.go       # Signal struct
internal/signal/detector.go    # Detector interface, RunAll()
internal/signal/text.go        # shared text utilities (incl. ErrorClass)
internal/signal/*.go           # one file per signal detector
internal/patterns/patterns.go  # cross-session clustering (Find → []Cluster)
internal/patterns/seed.go      # shared contract fixture (SeedContractFixture)
internal/rule/types.go         # tracers.rule/v1 record (Bundle, Card, …)
internal/rule/synth.go         # deterministic cluster → Card synthesis
internal/rule/redact.go        # evidence excerpt cap + secret pre-redactor
internal/signal/phrases/*.txt  # embedded phrase lists
testdata/                      # fixture transcripts
.github/workflows/ci.yml        # build + test on push/PR
.github/workflows/release.yml   # tag-triggered binary releases
.github/workflows/release-smoke.yml  # post-release binary + install.sh smoke tests
```
