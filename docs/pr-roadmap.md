# PR roadmap — source of truth for looptap + tracers

**Audience:** agents implementing Phase 1 across both repos.

**Pin this PR until merged:** use the branch commit SHA for fixtures — not `main` — while review is in flight.

| Read first | Purpose |
|------------|---------|
| [pr-roadmap.md](pr-roadmap.md) | **This file** — PR order, prompts, CI gates |
| [hybrid-architecture.md](hybrid-architecture.md) | Contracts 0–4 (schemas, argv, state machine) |
| [build-strategy.md](build-strategy.md) | Boundaries vs delegation, two-DB model |
| [tracers-scaffold.md](tracers-scaffold.md) | tracers file-level map (API routes, config keys) |
| [schemas/](schemas/) | JSON Schema stubs |
| [../testdata/contracts/](../testdata/contracts/) | Golden fixtures — looptap owns, tracers copies |

**Division of labor:** looptap = engine · tracers = edge + UX · Modal (`deploy/`) = LLM polish.

**Naming:** workflow rows are **`insights`** (`insights.db`, `/api/insights`) — not *findings* (`looptap analyze`) or *signals* (turn-level detectors).

---

## Contract ownership

looptap is the **producer**; tracers and Modal are **consumers**.

```
docs/schemas/              ← canonical JSON Schema (never fork in tracers)
testdata/contracts/        ← golden JSON bytes (copy into tracers/test/fixtures/looptap/)
```

**Sync rule:** contract bump = looptap PR first → tracers PR copies fixtures with header `# synced from looptap @ <sha>`. Treat golden diffs as contract breaks, not test fixes.

---

## PR sequence

| PR | Repo | Title | Depends on | Blocks |
|----|------|-------|------------|--------|
| **0** | looptap | Contract pack v1 | — | tracers 0b+ |
| **0b** | tracers | Parse golden fixtures | looptap 0 | tracers 1 |
| **1** | tracers | `insights.db` + mock ingest + HTMX | 0b | 2 |
| **2** | tracers | Redact before write | 1 | **3** |
| **3** | tracers | looptap subprocess chain | **2** | 5 |
| **4** | looptap | Modal `POST /v1/analyze` | 0 | 5 (real URL) |
| **5** | tracers | Analyze client + state machine | 3; 4 optional (mock OK) | — |
| **6+** | either | Rescan polish, share/sign, watcher | 5 | — |

**Hard rule:** PR **3 must not merge without PR 2**. First real looptap ingest requires redact-at-ingest.

---

## Parallel lanes (after looptap PR 0 merges)

```
looptap                          tracers
───────                          ───────
PR 0 contract pack (merge)  ──►  PR 0b parse tests
PR 4 Modal /v1/analyze      ║    PR 1 mock UI + insights.db
                             ║    PR 2 redact-at-ingest
                             ║    PR 3 subprocess
                             └──► PR 5 analyze (mock Modal until PR 4 lands)
```

---

## CI matrix

Add these tests in the same PR as the feature — not as follow-ups.

| Contract | looptap | tracers | Modal |
|----------|---------|---------|-------|
| `tracers.rule/v1` card | `internal/rule/types_test.go` golden round-trip | parse `golden-card.json` | — |
| `tracers.rule/v1` bundle | parse `golden-bundle.json`; `cards: []` never null | parse bundle + empty bundle | — |
| Schema validation | validate fixtures vs `docs/schemas/*.json` | — | — |
| Redaction | `internal/rule/redact_test.go` (pre-pass, non-authoritative) | ingest `leaky-bundle.json` → no `sk-ant` in DB | — |
| Subprocess argv | — | mock binary; exact argv; no shell | — |
| Analyze request | validate `analyze-request.golden.json` | build from card; strip `session_id`; ≤5 items | POST golden → 200 |
| Analyze response | — | merge mock; reject widened evidence | Pydantic + fingerprint |
| State machine | — | `detected → analyzing → proposed`; rescan skips `ignored` | — |

---

## Agent prompts

Copy verbatim into a new PR. Do not invent schema fields.

### looptap PR 0 — Contract pack v1

```
Repo: looptap
Task: Contract pack v1 — fixtures + schema CI.

1. Fixtures live in testdata/contracts/ (already seeded in PR #25).
2. Add internal/rule/contract_test.go:
   - Load each testdata/contracts/*.json
   - Validate against docs/schemas/*.json (jsonschema lib earns its keep here)
   - TestPatternsCmd output structurally matches golden-bundle (ignore generated_at)
3. Keep internal/rule/types_test.go reading golden-card.json (single source).

Do not rename JSON fields. Golden drift = contract break.
Run: go test ./...
```

### tracers PR 0b — Contract consumer

```
Repo: tracers
Task: Parse tracers.rule/v1 goldens (no DB, no UI).

1. Copy testdata/contracts/ → test/fixtures/looptap/ (header: synced from looptap @ <sha>).
2. Tests: parseBundle(golden-bundle) → 1 card; parseBundle(empty-bundle) → 0 cards;
   reject wrong schema string.
3. Wire into zig build test / CI.

Reference: tracers-web/src/rules.zig. No ingest yet.
```

### tracers PR 1 — Mock edge UI

```
Repo: tracers
Task: insights.db + mock bundle ingest + HTMX list.

Schema: hybrid-architecture.md Contract 2 (insights table, state CHECK).
Ingest: parse golden-bundle.json → upsert detected (card_json, source, session_count).
API: GET /api/insights?bucket=new. Minimal HTMX page.

Tests: migration applies; mock ingest → row count; API returns list.
No looptap subprocess. No redact.zig yet (fake data only).
```

### tracers PR 2 — Redact at ingest

```
Repo: tracers
Task: redact.zig on every evidence[].excerpt BEFORE INSERT.

Wire into ingest from PR 1. Use leaky-bundle.json fixture.
Test: stored card_json contains [REDACTED], not sk-ant; redactions > 0.

BLOCKS PR 3. Reference: build-strategy.md Step 2.
```

### tracers PR 3 — looptap subprocess

```
Repo: tracers
Task: std.process.Child chain — run → signal → patterns --format json.

Paths from config [looptap] only; reject \\0, newline, .. in db path.
Stdout → ingest + redact path from PR 2.

Test: mock looptap binary returns golden-bundle; assert exact argv per phase; no /bin/sh -c.
Reference: hybrid-architecture.md Contract 0, tracers-scaffold.md §1.
```

### looptap PR 4 — Modal /v1/analyze

```
Repo: looptap (deploy/)
Task: POST /v1/analyze — Pydantic in, Pydantic out.

Accept: tracers.analyze/v1 request schema. Return response schema.
Max 5 redacted_evidence; no session_id. Modal polishes rule only — evidence bytes
must match request fingerprint.

Tests: analyze-request.golden.json → valid response; oversized evidence → 422;
widened evidence in response → reject.
```

### tracers PR 5 — Analyze flow

```
Repo: tracers
Task: POST /api/insights/:id/analyze → Modal → merge → proposed.

Build request: strip session_id, cap 5 evidence, include template_rule.
Merge: keep local evidence; replace rule if source==llm; id must match card_id.
On failure: state=failed, template card preserved.

Tests: mock Modal golden response → proposed; widened evidence → failed.
UI: Analyze button + spinner (HTMX poll OK). Mock Modal fine until looptap PR 4 lands.
```

---

## Phase 1 exit criteria

One failure pattern flows **detect → analyze → display** with no raw secrets in Modal logs or `insights.db`.

- [ ] Golden fixtures parse in both repos' CI
- [ ] Mock ingest → redacted `insights.db` → analyze mock → `proposed`
- [ ] Real rescan via looptap subprocess (after PR 2 + 3)
- [ ] Modal stub returns `rule.source = "llm"` (after PR 4 + 5)

---

## Enhancements (after skeleton)

| Item | Touches contract? |
|------|-------------------|
| Better clustering / detectors | No — same bundle schema |
| JCS + Ed25519 + `expires_at` on share | Yes — share schema only |
| File watcher + debounced rescan | No — same ingest path |
| Port engine to Zig (Phase 3) | Retire subprocess; keep `insights.db` |
