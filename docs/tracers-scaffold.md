# tracers Phase 1 scaffold — implementation handoff

**Audience:** agents building in the [tracers](https://github.com/TanGentleman/tracers) repo.

**Source of truth (both repos):** [pr-roadmap.md](pr-roadmap.md) on looptap PR #25 — PR order, prompts, CI gates, fixture sync.

**Read first (in order):**

1. [pr-roadmap.md](pr-roadmap.md) — which PR to build, acceptance criteria, copy-paste prompt
2. [user-stories.md](user-stories.md) — what users see vs what stays opaque
3. [hybrid-architecture.md](hybrid-architecture.md) — contracts 0–4
4. [schemas/](schemas/) + [testdata/contracts/](../testdata/contracts/) — copy fixtures into `test/fixtures/looptap/`

**looptap side (shipped):** `looptap patterns --format json` → `tracers.rule/v1`. Golden card in [testdata/contracts/tracers.rule.v1.golden-card.json](../testdata/contracts/tracers.rule.v1.golden-card.json). Parser: tracers `tracers-web/src/rules.zig`.

---

## Division of labor

| Repo | Owns |
|------|------|
| **looptap** | Parse JSONL → SQLite → signals → patterns → bundle JSON on stdout |
| **tracers** | Subprocess orchestration, ingest, `insights` state machine, redaction, UI, signing |
| **Modal** (looptap `deploy/`) | `POST /v1/analyze` — LLM polish on redacted evidence only |

tracers does **not** reimplement clustering in Phase 1. Shell out to looptap.

---

## Phase 1 exit criteria

One failure pattern flows **detect → analyze → display** with no raw secrets in Modal logs or `insights.db`. Checklist: [pr-roadmap.md § Phase 1 exit criteria](pr-roadmap.md#phase-1-exit-criteria).

---

## Build order → PR map

| PR | This doc section | Notes |
|----|------------------|-------|
| 0b | §2 parse only | Copy fixtures; no DB yet |
| 1 | §3 workflow SQLite, §4 API, §6 UI | Mock ingest; redact optional for fake data |
| 2 | §2 ingest redact | **Blocks subprocess** |
| 3 | §1 subprocess | `run → signal → patterns` |
| 5 | §5 analyze, §4 `POST …/analyze` | Mock Modal OK until looptap PR 4 |

### 1. Subprocess wrapper (`tracers/src/looptap.zig` or extend `root.zig`) — **PR 3**

Fixed-argv chain — **never shell**:

```zig
try runLooptapPhase(init, &.{ "looptap", "run", "--db", db_path });
try runLooptapPhase(init, &.{ "looptap", "signal", "--db", db_path });
const bundle_json = try runLooptapCapture(init, &.{
    "looptap", "patterns", "--format", "json", "--db", db_path,
});
```

- Paths from `~/.tracers/config.toml` only; reject `\0`, newlines, `..`
- Replace today's `run → info → query` path with the patterns workflow

### 2. Bundle ingest — **PR 1 + PR 2**

Reuse `parseBundle` / `isSupported` from `tracers-web/src/rules.zig` (or move to `tracers/src/`).

Per card:

1. Reject if `schema != "tracers.rule/v1"`
2. **`redact.zig` every `evidence[].excerpt`** — update `redactions` count (**PR 2**)
3. Serialize scrubbed card → `card_json`
4. Upsert `insights` where `id = card.id`, state `detected` (skip `addressed` / `ignored` on rescan)

Fixture: [leaky-bundle.json](../testdata/contracts/tracers.rule.v1.leaky-bundle.json) for redaction CI.

### 3. Workflow SQLite — **PR 1**

Schema from [hybrid-architecture.md § Contract 2](hybrid-architecture.md#contract-2-state-machine-tracers-sqlite):

```sql
CREATE TABLE insights (
    id              TEXT PRIMARY KEY,
    state           TEXT NOT NULL CHECK (state IN (
                        'detected', 'analyzing', 'proposed',
                        'addressed', 'ignored', 'failed'
                    )),
    card_json       TEXT NOT NULL,
    source          TEXT NOT NULL,
    session_count   INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    analyzed_at     TEXT,
    addressed_at    TEXT
);
```

**Separate from looptap.db** — engine data vs workflow state.

### 4. Loopback API (`tracers serve`)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/insights` | List DTOs; `?bucket=new\|ready\|done` |
| `GET` | `/api/insights/:id` | Full scrubbed card |
| `POST` | `/api/insights/:id/analyze` | → `analyzing`; Modal; → `proposed` |
| `POST` | `/api/insights/:id/address` | Append snippet; → `addressed` |
| `POST` | `/api/insights/:id/dismiss` | → `ignored` |
| `POST` | `/api/insights/:id/share` | Re-redact, JCS sign, mint link |
| `POST` | `/api/rescan` | looptap chain + ingest |

Bearer auth on mutating routes (same as digest today).

### 5. Analyze client (Modal) — **PR 5**

Request: [tracers.analyze.v1.request.golden.json](../testdata/contracts/tracers.analyze.v1.request.golden.json). Strip `session_id`, max 5 evidence, include `template_rule`.

Merge per [hybrid-architecture.md § Merge rules](hybrid-architecture.md#merge-rules-tracers-side). On failure: `state = failed`.

### 6. UI (`tracers-web`) — **PR 1, 5**

Insights panel: HTMX poll `/api/insights`. Actions: Analyze | Save | Share | Dismiss.

### 7. Config (`~/.tracers/config.toml`)

```toml
[looptap]
db = "~/.looptap/looptap.db"
bin = "looptap"

[modal]
analyze_url = "https://…/v1/analyze"

[insights]
db = "~/.tracers/insights.db"
```

---

## Security checklist

- [ ] `redact.zig` before **any** `insights.card_json` write (PR 2)
- [ ] Fixed argv for looptap — no shell (PR 3)
- [ ] Analyze: no `session_id`, ≤5 evidence (PR 5)
- [ ] Share: JCS + Ed25519; `expires_at` in signed bytes (Phase 2+)
- [ ] Loopback-only serve; bearer auth on mutating routes

---

## Tests → CI matrix

Full matrix: [pr-roadmap.md § CI matrix](pr-roadmap.md#ci-matrix).

| Test | PR |
|------|-----|
| Parse golden bundle + empty bundle | 0b |
| Ingest + redact leaky fixture | 2 |
| Mock looptap subprocess argv | 3 |
| State machine transitions | 5 |
| Analyze merge rejects widened evidence | 5 |

---

## Cross-link in tracers repo

Add to tracers `docs/looptap-handoff.md`:

```markdown
Source of truth: https://github.com/TanGentleman/looptap/blob/<sha>/docs/pr-roadmap.md
Fixtures: copy testdata/contracts/ → test/fixtures/looptap/
```
