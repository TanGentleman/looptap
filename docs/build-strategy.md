# Build strategy: boundaries first, agents inside them

Actionable plan for the hybrid stack. **You** nail the contracts and trust boundaries; **agents** implement components inside those boxes.

Companion docs: [hybrid-architecture.md](hybrid-architecture.md) (contracts), [tracers-scaffold.md](tracers-scaffold.md) (Phase 1 checklist), [user-stories.md](user-stories.md) (flows).

## Naming: why `insights`, not `findings`

Workflow rows in tracers are **`insights`** — a scrubbed `tracers.rule/v1` card plus UI state (`detected` → `proposed` → …).

| Term | Means | Owner |
|------|-------|-------|
| **signal** | One turn- or session-level hit (`failure`, `loop`, …) | looptap |
| **pattern** | Cross-session cluster metadata inside a card | looptap → card |
| **card** | Wire record: pattern + evidence + rule | `tracers.rule/v1` |
| **insight** | Card persisted in tracers + workflow state | tracers `insights.db` |

Avoid **findings** — collides with `looptap analyze` output. Avoid reusing **signals** for the workflow table.

---

## 1. What to iron out vs. delegate

### You (architect) lock down manually

| Boundary | Why |
|----------|-----|
| **JSON Schemas** | Agents invent fields without strict `tracers.rule/v1`, analyze, and share schemas |
| **Subprocess invocation** | Exact argv, path sanitization, no shell — see [hybrid-architecture.md § Contract 0](hybrid-architecture.md) |
| **Redaction rules** | What counts as a secret; `redact.zig` is authoritative; map looptap pre-pass to same classes |
| **`insights` schema + state machine** | Table shape and transitions `detected → analyzing → proposed → addressed \| ignored` |
| **Two-database split** | looptap owns `looptap.db`; tracers owns `insights.db` — see below |

### Agents implement inside those boundaries

| Task | Prompt shape |
|------|----------------|
| Zig subprocess wrapper | "`std.process.Child` with this exact argv; capture stdout; reject non-zero exit" |
| Go bundle emission | "Marshal these structs to match `schemas/tracers.rule/v1.json`" (mostly done — verify golden test) |
| Modal `/v1/analyze` | "FastAPI + Pydantic: accept `tracers.analyze/v1.request.json`, return response schema; max 5 evidence items" |
| tracers ingest + UI | "Parse bundle → redact excerpts → upsert `insights` → serve HTMX list from `/api/insights`" |

---

## 2. SQLite: two files, no shared writers

**Do not** have looptap and tracers write the same SQLite file. Subprocess handoff avoids WAL locking, cross-binary races, and "who migrated the schema?" fights.

```
┌─────────────────┐     stdout JSON      ┌─────────────────┐
│  looptap.db     │ ──────────────────►  │  insights.db    │
│  (engine cache) │   tracers.rule/v1    │  (workflow)     │
│  Go writes ONLY │   Bundle on stdout   │  Zig writes ONLY│
└─────────────────┘                      └─────────────────┘
        ▲                                          ▲
        │ looptap run/signal/patterns              │ ingest + state
        │ (subprocess, fixed argv)                 │ + UI + Modal client
   tracers orchestrates                     tracers never opens looptap.db
```

| File | Writer | Reader | Contents |
|------|--------|--------|----------|
| `~/.tracers/looptap.db` (or config path) | looptap only | looptap only | sessions, turns, signals |
| `~/.tracers/insights.db` | tracers only | tracers only | scrubbed cards + workflow state |

Flow: tracers runs `looptap patterns --format json --db <path>` → reads **stdout** → redacts → upserts **`insights`**. tracers never `sqlite3_open`s looptap's file.

This resolves open decision #1 in [hybrid-architecture.md](hybrid-architecture.md): **Option A** — tracers-owned paths, looptap invoked with `--db` for engine work; workflow lives in a separate file.

---

## 3. Walking skeleton (E2E prioritization)

Build the pipeline end-to-end with mocks first, then swap in real pieces. Order matters for security: **redact before the first real row hits `insights.db`**, not as a later hardening pass.

### Step 1 — Mocked edge UI (days 1–2)

**Goal:** Zig serve + `insights.db` + HTMX UI without Go or Modal.

**You provide:** `mock_bundle.json` matching [schemas/tracers.rule.v1.json](schemas/tracers.rule.v1.json).

**Agent task:**

- Parse bundle → upsert `insights` (`state = detected`)
- `GET /api/insights` + simple HTML/HTMX list

**Done when:** Local UI shows mock cards in the **New** bucket.

### Step 2 — Wire looptap subprocess (days 3–4)

**Goal:** Real `.jsonl` → UI without mock file.

**You provide:** Frozen argv list and config keys ([tracers-scaffold.md](tracers-scaffold.md)).

**Agent task:**

- Replace file-read with `std.process.Child`: `run` → `signal` → `patterns --format json`
- Parse stdout with existing `rules.zig` parser

**Note:** `looptap patterns --format json` and golden tests already exist in this repo — Step 2 is mostly **tracers wiring**, not new Go types.

**Done when:** New sessions appear in UI after rescan.

### Step 3 — Authoritative redaction at ingest (day 5)

**Goal:** `insights.db` and UI never store raw secrets.

**You provide:** Redaction class list (mirror `tracers/src/redact.zig` + looptap `redact.go`).

**Agent task:**

- Run `redact.zig` on every `evidence[].excerpt` **before** `INSERT` into `insights`
- Table test: fixture with `sk-…` in excerpt → stored JSON shows `[REDACTED]`

**Done when:** Leaking `insights.db` or XSS in UI cannot exfiltrate keys. Do not defer this past Step 2's first real ingest if you can help it.

### Step 4 — Modal analyze (days 6–7)

**Goal:** **Analyze** button → LLM polish → **Ready to apply**.

**You provide:** Modal stub URL; [tracers.analyze.v1](schemas/tracers.analyze.v1.request.json) schemas.

**Agent task:**

- Pydantic-validated `/v1/analyze` (≤5 evidence turns, no `session_id`)
- UI: `POST /api/insights/:id/analyze` → `analyzing` → merge → `proposed`

**Done when:** Full path: JSONL → Go → Zig redact → UI → Modal → updated snippet. Phase 1 exit criteria met.

### Step 5 — Enhancements (after skeleton works)

Safe to parallelize once Step 4 is green:

| Enhancement | Touches boundary? |
|-------------|-------------------|
| Better looptap clustering / detectors | No — same bundle schema |
| JCS + Ed25519 + `expires_at` on share | Yes — share schema only |
| Richer Modal prompts | No — analyze envelope unchanged |
| File watcher + debounced rescan | No — same ingest path |
| Port engine to Zig (Phase 3) | Retire subprocess, keep `insights.db` |

---

## 4. Agent handoff checklist

Before spawning implementers, confirm:

- [ ] Schemas committed under `docs/schemas/`
- [ ] `insights` table + state names frozen
- [ ] Two DB files documented in config
- [ ] argv template for looptap chain (no shell)
- [ ] Redaction-at-ingest in acceptance criteria for Step 2/3

Then point agents at [tracers-scaffold.md](tracers-scaffold.md) for file-level tasks.
