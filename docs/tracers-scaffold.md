# tracers Phase 1 scaffold — implementation handoff

**Audience:** agents and humans building in the [tracers](https://github.com/TanGentleman/tracers) repo.

**Read first (in order):**

1. [user-stories.md](user-stories.md) — what users see vs what stays opaque
2. [hybrid-architecture.md](hybrid-architecture.md) — contracts 0–4 and phase plan
3. [schemas/](schemas/) — JSON Schema stubs (`tracers.rule/v1`, analyze, share)

**looptap side (done):** `looptap patterns --format json` emits `tracers.rule/v1` bundles. Golden test in `internal/rule/types_test.go` pins the wire format to tracers' [rule-with-evidence.md](https://github.com/TanGentleman/tracers/blob/main/docs/rule-with-evidence.md).

---

## Division of labor

| Repo | Owns |
|------|------|
| **looptap** | Parse JSONL → SQLite → signals → patterns → bundle JSON on stdout |
| **tracers** | Subprocess orchestration, ingest, `findings` state machine, redaction, UI, signing, share mint |
| **Modal** (looptap `deploy/`) | `POST /v1/analyze` — LLM polish on redacted evidence only |

tracers does **not** reimplement clustering in Phase 1. Shell out to looptap.

---

## Phase 1 exit criteria

One failure pattern flows **detect → analyze → display** with no raw secrets in Modal logs or `findings.db`.

---

## Build order (suggested)

### 1. Subprocess wrapper (`tracers/src/looptap.zig` or extend `root.zig`)

Fixed-argv chain — **never shell**:

```zig
// After config resolves db_path:
try runLooptapPhase(init, &.{ "looptap", "run", "--db", db_path });
try runLooptapPhase(init, &.{ "looptap", "signal", "--db", db_path });
const bundle_json = try runLooptapCapture(init, &.{
    "looptap", "patterns", "--format", "json", "--db", db_path,
});
```

- Paths from `~/.tracers/config.toml` only; reject `\0`, newlines, `..`
- Mirror existing `invokeLooptap` in `root.zig` but add `signal` + `patterns`
- Today: `runLooptap` does `run → info → query` — **replace/extend** for patterns workflow

### 2. Bundle ingest (`tracers-web/src/rules.zig` already parses)

Reuse `parseBundle` / `isSupported` from tracers-web or move parser to `tracers/src/` for core ingest.

Per card:

1. Reject if `schema != "tracers.rule/v1"`
2. **`redact.zig` every `evidence[].excerpt`** — update `redactions` count
3. Serialize scrubbed card → `card_json`
4. Upsert `findings` where `id = card.id`, state `detected` (skip `addressed` / `ignored` on rescan)

### 3. Workflow SQLite (`~/.tracers/findings.db` or alongside identity)

Schema from [hybrid-architecture.md § Contract 2](hybrid-architecture.md#contract-2-state-machine-tracers-sqlite):

```sql
CREATE TABLE findings (
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

Map internal states → user buckets (see [user-stories.md § Story 2](user-stories.md#story-2--see-insights-and-pipeline-stage)).

### 4. Loopback API (`tracers serve`)

Add authenticated endpoints (bearer token, same as digest today):

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/findings` | List display DTOs; `?bucket=new\|ready\|done` |
| `GET` | `/api/findings/:id` | Full scrubbed card |
| `POST` | `/api/findings/:id/analyze` | → `analyzing`; call Modal; merge → `proposed` |
| `POST` | `/api/findings/:id/address` | Append snippet; → `addressed` |
| `POST` | `/api/findings/:id/dismiss` | → `ignored` |
| `POST` | `/api/findings/:id/share` | Re-redact, JCS sign, mint link |
| `POST` | `/api/rescan` | Run looptap chain + ingest |

Existing transcript share stays: `POST /share/:id` → `/s/:token`.

### 5. Analyze client (Modal)

Build request per [schemas/tracers.analyze.v1.request.json](schemas/tracers.analyze.v1.request.json):

- Strip `session_id` from evidence
- **Max 5** evidence items (`maxItems` in schema)
- Include `template_rule` from card

Merge response per [hybrid-architecture.md § Merge rules](hybrid-architecture.md#merge-rules-tracers-side): keep local evidence; replace `rule` if `source == "llm"`.

On failure: `state = failed`, keep template card usable.

### 6. UI (`tracers-web`)

New **Insights** panel (HTMX poll `/api/findings` or proxy via serve):

- Row: title, session count, bucket badge, expand evidence
- Actions: Analyze | Save | Share insight | Dismiss
- `analyzing` → spinner until state changes

Keep flagged-sessions panel for **Share session** (transcript path).

### 7. Config (`~/.tracers/config.toml`)

```toml
[looptap]
db = "~/.looptap/looptap.db"
bin = "looptap"  # or pinned path

[modal]
analyze_url = "https://…/v1/analyze"
# API key stays in tracers env, never passed to looptap subprocess

[findings]
db = "~/.tracers/findings.db"
```

---

## Security checklist (non-negotiable)

- [ ] `redact.zig` before **any** `findings.card_json` write
- [ ] `std.process.Child` / fixed argv for all looptap calls — no shell
- [ ] Analyze request: no `session_id`, ≤5 evidence turns
- [ ] Share: JCS + Ed25519; `expires_at` in signed bytes ([share schema](schemas/tracers.share.v1.request.json))
- [ ] Loopback-only serve; bearer auth on mutating routes

---

## Tests to add (tracers)

| Test | What |
|------|------|
| `rules.zig` | Already covers bundle parse — keep green against looptap golden |
| Ingest + redact | Fixture bundle with fake API key in excerpt → stored card has `[REDACTED]`, `redactions > 0` |
| Subprocess | Mock looptap binary returning fixture JSON; assert argv has no shell |
| State machine | `detected → analyzing → proposed`; rescan skips `ignored` |
| Analyze merge | Modal mock returns widened evidence → rejected, state `failed` |

Seed fixture: run `looptap patterns --format json` on looptap testdata or use golden from `rules.zig` `sample` constant.

---

## What looptap is *not* building in Phase 1

- tracers UI, findings DB, file watcher
- Modal `/v1/analyze` endpoint (evolve `deploy/app.py` separately)
- Retiring Go binary (Phase 3)

---

## Cross-links in tracers repo

After scaffold lands, add to tracers `docs/looptap-handoff.md`:

```markdown
Cross-system contracts and Phase 1 scaffold:
https://github.com/TanGentleman/looptap/blob/main/docs/tracers-scaffold.md
```

Parser reference: `tracers-web/src/rules.zig` (already matches `tracers.rule/v1`).
