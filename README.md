looptap
looptap parses local coding agent transcripts (Claude Code, Codex, etc.), computes lightweight behavioral signals, and writes everything to a SQLite database. You point datasette at the DB for visualization.
The name: you're tapping into the feedback loop between developer and agent.

What it does
local transcript files → parse → SQLite → signal → SQLite (enriched)
                                                         ↓
                                                    datasette / any SQL client
Four commands:
Command	Purpose
looptap parse	Discover agent transcripts, normalize to common schema, write to SQLite. Incremental (skips unchanged files by hash).
looptap signal	Run signal detectors over parsed sessions, write results to signals table. Skips already-signaled sessions unless --recompute.
looptap run	parse then signal.
looptap info	Print DB stats (session/turn/signal counts by source and type).
Global flag: --db <path> (default ~/.looptap/looptap.db).

Language & dependencies
Go. Use the standard library where possible.
Dependency	Purpose
github.com/spf13/cobra	CLI framework
github.com/mattn/go-sqlite3	SQLite driver (CGo)
github.com/BurntSushi/toml	Config parsing
github.com/stretchr/testify	Test assertions
No web framework. No ORM. No LLM client libraries.

Project layout
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
│       ├── misalignment.go
│       ├── stagnation.go
│       ├── disengagement.go
│       ├── satisfaction.go
│       ├── failure.go
│       ├── loop.go
│       └── exhaustion.go
├── phrases/                       # go:embed, one phrase per line
│   ├── misalignment.txt
│   ├── disengagement.txt
│   └── satisfaction.txt
└── testdata/
    ├── claude_code/*.jsonl        # fixture transcripts (one per signal type)
    ├── codex/*.jsonl
    └── golden/*.signals.json      # expected signal output snapshots
All domain logic lives in internal/. The cmd/ layer is glue — it parses flags, calls internal/, and prints output. internal/ packages are importable as a Go library for server-side use.

Core types
// internal/parser/types.go

type Session struct {
    ID        string    // deterministic: sha256(Source + SessionID)
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
    ToolName string    // non-empty for tool_use and tool_result
    IsError  bool
}
// internal/signal/types.go

type Signal struct {
    SessionID string
    Type      string   // "misalignment", "stagnation", "disengagement",
                       // "satisfaction", "failure", "loop", "exhaustion"
    Category  string   // "interaction", "execution", "environment"
    TurnIdx   *int     // nil = session-level
    Confidence float64 // 0.0–1.0
    Evidence   string  // phrase or pattern that matched
}

Interfaces
Two interfaces. Each new agent or signal type = one file implementing one interface.
// internal/parser/parser.go

type Parser interface {
    Name() string
    CanParse(path string) bool
    Parse(path string) (Session, error)
}

// Detect returns the right parser for a file, or error.
func Detect(path string) (Parser, error)

// Discover walks dirs and returns parseable file paths.
func Discover(dirs []string) ([]string, error)
// internal/signal/detector.go

type Detector interface {
    Name() string                    // "misalignment", etc.
    Category() string                // "interaction", "execution", "environment"
    Detect(s parser.Session) []Signal
}

// RunAll runs every registered detector against a session.
func RunAll(s parser.Session) []Signal
Registration is a package-level slice, not a plugin system:
var All = []Detector{
    &Misalignment{},
    &Stagnation{},
    &Disengagement{},
    &Satisfaction{},
    &Failure{},
    &Loop{},
    &Exhaustion{},
}

SQLite schema
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
    evidence        TEXT,
    FOREIGN KEY (session_id, turn_idx) REFERENCES turns(session_id, idx)
);

CREATE INDEX idx_signals_type ON signals(signal_type);
CREATE INDEX idx_signals_session ON signals(session_id);
CREATE INDEX idx_sessions_source ON sessions(source);
CREATE INDEX idx_sessions_project ON sessions(project);
Schema migrations live in internal/db/db.go as versioned SQL strings applied on Open().

Signal detection rules
All detectors operate on a Session and its []Turn. No network calls. No LLM calls.
Misalignment (interaction): scan user turns for correction phrases from phrases/misalignment.txt (normalized, typo-tolerant via edit distance). Also flag when consecutive user turns have token similarity > 0.7 (user repeating themselves because the agent missed the point).
Stagnation (interaction): compute pairwise token similarity of assistant turns. Flag if any pair exceeds 0.8. Also flag sessions whose turn count exceeds 2× the median turn count for that project (requires prior sessions to exist).
Disengagement (interaction): check the final user turn — flag if it's ≤ 5 words and doesn't match satisfaction phrases. Also match abandonment phrases from phrases/disengagement.txt.
Satisfaction (interaction): match gratitude and success phrases from phrases/satisfaction.txt in the final 3 user turns.
Failure (execution): scan tool_result turns where IsError == true. Also match error patterns in content (stack traces, "command failed", "exit code").
Loop (execution): sliding window of size 6 over tool_use turns. Flag when ≥ 3 turns in the window share the same ToolName and content similarity > 0.8.
Exhaustion (environment): match rate-limit, context-length, and timeout patterns in tool_result and system turns. Patterns: "rate limit", "context length exceeded", "token limit", "timed out", "429".
Shared text utilities (signal/text.go)
func Normalize(s string) string           // lowercase, strip punctuation, collapse whitespace
func TokenSimilarity(a, b string) float64 // Jaccard similarity on whitespace-split tokens
func MatchPhrases(text string, phrases []string, maxEditDist int) (bool, string)
Phrase lists are loaded once at init via go:embed and cached. Users override via config:
[phrases]
misalignment = "/path/to/replace.txt"
misalignment_extra = "/path/to/append.txt"

Agent transcript formats
Claude Code
Location: ~/.claude/projects/<project-hash>/sessions/<session-id>.jsonl
JSONL, one JSON object per line. Each line has a type field. Key types:
* type: "human" → user turn. Content in message.content (string or array of content blocks).
* type: "assistant" → assistant turn. Content may include text blocks and tool_use blocks (with name, input).
* type: "tool_result" → tool output. Has tool_use_id, content (string or blocks), is_error.
Also has sessions-index.json per project with summaries, message counts, git branches.
Codex CLI
Location: ~/.codex/sessions/ (structure TBD — Codex stores transcripts as newline-delimited JSON events). Use codex exec --json output format as reference.
Events have type fields for user messages, assistant messages, tool calls, and tool results. Exact field names will need to be confirmed against a real transcript.

Testing
Signal detector tests (table-driven, one file per detector)
func TestMisalignment(t *testing.T) {
    tests := []struct {
        name     string
        turns    []Turn
        wantSigs int
    }{
        {"correction phrase", correctionTurns, 1},
        {"user rephrasing", rephraseTurns, 1},
        {"normal conversation", normalTurns, 0},
    }
    d := &Misalignment{}
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := d.Detect(Session{Turns: tt.turns})
            assert.Len(t, got, tt.wantSigs)
        })
    }
}
Parser tests (real JSONL fixtures)
func TestClaudeCodeParse(t *testing.T) {
    s, err := (&ClaudeCode{}).Parse("testdata/claude_code/simple_session.jsonl")
    require.NoError(t, err)
    assert.Equal(t, "claude-code", s.Source)
    assert.Equal(t, "user", s.Turns[0].Role)
}
DB round-trip tests
func TestInsertAndRetrieve(t *testing.T) {
    db := testDB(t)
    s := fixture(t, "testdata/claude_code/simple_session.jsonl")
    require.NoError(t, db.InsertSession(s))
    got, _ := db.GetSession(s.ID)
    assert.Equal(t, s.Source, got.Source)
}
Golden file tests (signal regression)
Run all detectors on each fixture, compare JSON output to testdata/golden/*.signals.json. Regenerate with go test -update.

Config
~/.looptap/config.toml:
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

Datasette usage
pip install datasette datasette-vega
datasette ~/.looptap/looptap.db --metadata datasette/metadata.yml
Ship a datasette/metadata.yml with canned queries (signal trends over time, most-signaled sessions, per-project breakdown). Users get browse, facet, chart, and JSON API out of the box.

Out of scope
* Custom web UI (use datasette)
* LLM-powered analysis (add later as a separate command)
* Real-time streaming (batch tool — run on a schedule)
* Multi-user auth (local-first; server-side multi-tenant is the wrapping service's job)
