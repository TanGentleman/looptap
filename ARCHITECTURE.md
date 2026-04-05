# Architecture

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

Implementations: `ClaudeCode` (working), `Codex` (stub).

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

**User messages:** `message.content` is either a string (plain text) or an array of content blocks (tool results).

**Assistant messages:** `message.content` is always an array — `text`, `tool_use`, and `thinking` blocks. Thinking blocks are skipped.

**Turn mapping:**
- User string content → `Role: "user"`
- User tool_result block → `Role: "tool_result"`, `IsError` from `is_error`
- Assistant text blocks → accumulated into one `Role: "assistant"` turn
- Assistant tool_use block → `Role: "tool_use"`, `ToolName` from `name`, `Content` is JSON of input

**Metadata:** `sessionId`, `cwd` (→ Project), `gitBranch`, `message.model`, timestamps from first/last lines. Session ID is `sha256("claude-code" + sessionId)`.

Subagent transcripts (in `<session>/subagents/`) are skipped.

## Signal detection rules

All detectors operate on `Session` + `[]Turn`. No network. No LLM.

| Signal | Category | Detection |
|--------|----------|-----------|
| **Misalignment** | interaction | Correction phrases from `phrases/misalignment.txt` (edit-distance tolerant). Consecutive user turns with token similarity > 0.7. |
| **Stagnation** | interaction | Pairwise assistant turn similarity > 0.8. Turn count > 2× project median. |
| **Disengagement** | interaction | Final user turn ≤ 5 words and not a satisfaction phrase. Abandonment phrases from `phrases/disengagement.txt`. |
| **Satisfaction** | interaction | Gratitude/success phrases from `phrases/satisfaction.txt` in final 3 user turns. |
| **Failure** | execution | `tool_result` turns with `IsError == true`. Error patterns in content (stack traces, "command failed", "exit code"). |
| **Loop** | execution | Sliding window of 6 over `tool_use` turns. Flag when ≥ 3 share the same ToolName with content similarity > 0.8. |
| **Exhaustion** | environment | Rate-limit/context-length/timeout patterns in `tool_result` and `system` turns. |

## Text utilities (`internal/signal/text.go`)

```go
func Normalize(s string) string           // lowercase, strip punctuation, collapse whitespace
func TokenSimilarity(a, b string) float64 // Jaccard similarity on whitespace-split tokens
func MatchPhrases(text string, phrases []string, maxEditDist int) (bool, string)
```

`MatchPhrases` does exact substring match first, then falls back to Levenshtein edit distance on word-level sliding windows.

## Advisor (`internal/advise/`)

The `advise` command closes the loop: signals go in, CLAUDE.md rules come out.

```
SQLite signals → context.go (gather) → prompt.go (assemble) → llm.go (Gemini) → parse JSON → print
```

**`context.go`** — SQL queries that pull signal summaries + failure/loop/misalignment details. Returns structs, not strings.

**`prompt.go`** — System prompt tells the model to output a JSON array of recommendations. User prompt builder assembles signal context into labeled sections.

**`llm.go`** — Thin wrapper around `genai.Client.Models.GenerateContent`. This is the only file that imports the Gemini SDK — the swap point if adk-go or another framework earns its keep later.

**`advise.go`** — Orchestrator. Gather → prompt → call → parse → return. No cobra knowledge, no `os.Exit`.

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

[advise]
api_key = ""                               # prefer GOOGLE_API_KEY env
model = "gemini-3.1-flash-lite-preview"
```

## Dependencies

| Dependency | Why |
|------------|-----|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/mattn/go-sqlite3` | SQLite driver (CGo) |
| `github.com/BurntSushi/toml` | Config parsing |
| `github.com/stretchr/testify` | Test assertions |
| `google.golang.org/genai` | Gemini API client (for `advise`) |

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
internal/parser/codex.go       # Codex parser (stub)
internal/signal/types.go       # Signal struct
internal/signal/detector.go    # Detector interface, RunAll()
internal/signal/text.go        # shared text utilities
internal/signal/*.go           # one file per signal detector
internal/advise/advise.go      # advisor orchestrator
internal/advise/context.go     # signal context queries
internal/advise/prompt.go      # LLM prompt templates
internal/advise/llm.go         # Gemini API wrapper
internal/advise/types.go       # Recommendation, AdviceResult
phrases/*.txt                  # embedded phrase lists
testdata/                      # fixture transcripts
```
