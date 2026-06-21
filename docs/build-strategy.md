# Build strategy: boundaries first, agents inside them

Actionable plan for the hybrid stack. **You** nail the contracts and trust boundaries; **agents** implement components inside those boxes.

**Agents start here:** [pr-roadmap.md](pr-roadmap.md) — PR order, copy-paste prompts, CI matrix, parallel lanes.

Companion docs: [hybrid-architecture.md](hybrid-architecture.md) (contracts), [tracers-scaffold.md](tracers-scaffold.md) (tracers file map), [user-stories.md](user-stories.md) (flows).

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

See [pr-roadmap.md § Agent prompts](pr-roadmap.md#agent-prompts) for copy-paste tasks per PR.

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

---

## 3. Walking skeleton (E2E prioritization)

Build the pipeline end-to-end with mocks first, then swap in real pieces. **Redact before the first real row hits `insights.db`** — not as a later hardening pass.

PR numbers map to [pr-roadmap.md](pr-roadmap.md).

| Step | PR(s) | Goal | Done when |
|------|-------|------|-----------|
| **0** Contract pack | looptap 0 | Fixtures + schema CI | Goldens validate; siblings can pin SHA |
| **1** Mock edge UI | tracers 0b, 1 | `insights.db` + HTMX without Go/Modal | Mock cards in **New** bucket |
| **2** Redact at ingest | tracers 2 | No raw secrets in `insights.db` | `leaky-bundle.json` → `[REDACTED]` in DB |
| **3** looptap subprocess | tracers 3 | Real `.jsonl` → UI | Rescan populates UI (**requires Step 2**) |
| **4** Modal analyze | looptap 4, tracers 5 | Analyze → **Ready to apply** | Full path with no secrets in Modal logs |
| **5** Enhancements | 6+ | Share, watcher, detectors | Parallel after Step 4 green |

**looptap engine (shipped):** `patterns --format json`, golden test in `internal/rule/types_test.go`, fixtures in [testdata/contracts/](../testdata/contracts/).

---

## 4. Agent handoff checklist

Before spawning implementers, confirm:

- [x] Schemas under [docs/schemas/](schemas/)
- [x] Golden fixtures under [testdata/contracts/](../testdata/contracts/)
- [x] PR roadmap with prompts — [pr-roadmap.md](pr-roadmap.md)
- [ ] `insights` table migrated in tracers (PR 1)
- [ ] argv template for looptap chain (PR 3)
- [ ] Redact-at-ingest green before subprocess merge (PR 2 blocks PR 3)

Then assign PRs from [pr-roadmap.md](pr-roadmap.md).
