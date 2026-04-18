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

## Analyzer (`internal/analyze/`)

The `analyze` command is `advise`'s mirror image: instead of asking "what rules should you add?", it asks "how good are the rules you already have?". Same Gemini wrapper, different prompt.

```
CLAUDE.md → reader.go → prompt.go (assemble) → advise.Client → parse JSON → print
```

**`reader.go`** — Reads the target file (default `~/.claude/CLAUDE.md`). User-facing errors for missing/empty files.

**`prompt.go`** — System prompt frames the LLM as a quality reviewer (clarity, completeness, consistency, structure, actionability) and asks for findings inside a ```json fence.

**`analyze.go`** — Orchestrator. Reuses `advise.NewClient` rather than duplicating the Gemini wrapper. Strips the ```json fence, parses, returns `Finding`s.

## HTML report (`internal/htmlreport/`)

The `html` command points a coding agent at a git branch and asks it to write a shareable story for the rest of the team. Unlike `advise` and `analyze`, the LLM is not called via an SDK — it's a plain subprocess invocation of the agent's CLI in print mode. Two agents are supported: `claude` (Claude Code, the default) and `opencode` (sst/opencode).

```
HTMLSettings → resolve.go (filesystem + git + agent) → Resolved → generate.go (agent CLI) → HTML string
```

**`types.go`** — `HTMLSettings` is the user-facing knob bag: `RepoPath`, `BranchMode` (`current` | `default` | `custom`), `BranchName`, `Agent` (`claude` | `opencode`), `OpencodeConfigPath`. `Resolved` is the post-validation shape — absolute repo path, concrete branch name, concrete agent, absolute opencode config path — and everything downstream reads from it. Keep `HTMLSettings` small; model name, tone, section toggles slot in as they earn their keep (or land in the opencode config file for the opencode path).

**`resolve.go`** — Verifies the repo path exists, is a directory, and is actually a git repo (`git rev-parse --show-toplevel`). Then resolves the branch: `current` reads HEAD and errors on detached state; `default` tries `origin/HEAD` and falls back to `main`/`master`; `custom` verifies the branch exists locally or on `origin`. `ParseBranchFlag` turns the raw `--branch`/`LOOPTAP_BRANCH` string into a `(mode, name)` pair; `ParseAgentFlag` does the same for `--agent`/`LOOPTAP_AGENT`. For opencode, `resolveAgentConfig` also asserts that `OpencodeConfigPath` points at a real file and makes it absolute.

**`prompt.go`** — System append (forces single-HTML output) and user prompt builder (branch analysis steps + output constraints). Both prompts are agent-agnostic; the agent-specific wrapping lives in `generate.go`.

**`generate.go`** — Shells out to the agent via an injectable `Runner` seam:

```go
type Runner func(ctx context.Context, dir string, args []string) (string, error)
func Generate(ctx context.Context, r *Resolved, runner Runner) (string, error)
```

A nil runner uses the real agent binary on PATH for `r.Agent`; tests pass a fake. `buildArgsFor(r)` picks between `buildClaudeArgs` and `buildOpencodeArgs`.

Claude invocation:

```
claude -p <prompt>
  --output-format text
  --permission-mode bypassPermissions
  --allowedTools Bash,Read,Glob,Grep
  --append-system-prompt <HTML-only instruction>
  --max-turns 40
```

Override the binary with `LOOPTAP_CLAUDE_BIN`.

Opencode invocation:

```
opencode run <prompt> --dangerously-skip-permissions
```

With `OPENCODE_CONFIG=<absolute path to JSON>` set in the subprocess env. Allowed tools, model, provider credentials, and system prompt all live in that config file — opencode's `run` subcommand doesn't expose `--allowedTools` or `--append-system-prompt`, so the JSON is the knob bag. The strict HTML-only instruction (normally claude's `--append-system-prompt`) is folded into the user prompt for opencode because its `--system` flag *overrides* rather than appends and would clobber the config's system prompt. Override the binary with `LOOPTAP_OPENCODE_BIN`.

**Default config (`defaults.go` + `opencode.default.json`)** — when `--opencode-config` is unset, `runOpencode` materializes the embedded `DefaultOpencodeConfig` bytes to a tempfile for the duration of the call. The default is a read-only shape matched to branch-inspection: `read`/`glob`/`grep`/`list`/`bash` allow, `edit`/`webfetch`/`websearch` deny. Schema follows https://opencode.ai/config.json: top-level `$schema`, `model` (`provider/model-id`), and `permission` (object of per-tool `"allow"|"deny"|"ask"` actions; `write`/`edit`/`patch`/`multiedit` all fold into `permission.edit`; `bash` can also take a nested pattern map). `TestDefaultOpencodeConfig_Schema` locks the shape down so a careless edit to the JSON fails at `go test`, not at runtime.

The working directory is set to `r.RepoPath`, so git Just Works. The prompt walks the agent through finding the base branch, reading the diff and changed files, and writing narrative + commits + files + risks into a single self-contained `<!doctype html>…</html>` document with inline CSS. `stripFences` forgives a stray ```html wrapper; `looksLikeHTML` rejects anything that doesn't look like a document so we fail loudly instead of writing plain text to disk.

The cobra wiring in `cmd/html.go` stays thin: flag/env resolution, confirmation prompt (skipped by `--force`), one `Generate` call, then either stdout or `-o file`.

## Prompt convention

Every package that talks to an LLM keeps its prompts in `prompt.go`. System prompts are `const` strings; user prompts are assembled by a `buildPrompt` or `BuildUserPrompt` function. This means "find every prompt" = `grep -l systemPrompt internal/*/prompt.go`.

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
| `google.golang.org/genai` | Gemini API client (for `advise` and `analyze`) |

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
internal/analyze/analyze.go    # CLAUDE.md quality reviewer
internal/analyze/reader.go     # file I/O + default path resolution
internal/analyze/prompt.go     # quality-review prompt templates
internal/analyze/types.go      # Finding, AnalyzeResult
internal/htmlreport/types.go     # HTMLSettings, Resolved, BranchMode
internal/htmlreport/resolve.go   # repo + branch validation
internal/htmlreport/prompt.go    # HTML report prompt templates
internal/htmlreport/generate.go  # claude -p subprocess invocation
phrases/*.txt                  # embedded phrase lists
testdata/                      # fixture transcripts
.github/workflows/ci.yml        # build + test on push/PR
.github/workflows/release.yml   # tag-triggered binary releases
.github/workflows/release-smoke.yml  # post-release binary + install.sh smoke tests
```
