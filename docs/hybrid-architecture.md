# Hybrid Architecture: looptap + tracers + Modal

Cross-system contract for the pipeline that moves a transcript from a raw JSONL file to a signed, shareable insight. looptap stays the **deterministic engine** (silent engine room — data extraction); tracers is the **trusted edge** (secure edge and UX — state machine, redaction, signing, UI); Modal is the **stateless LLM polish** layer (stateless brain — synthesis on redacted evidence only).

For looptap-only internals (parsers, detectors, SQLite schema), see [ARCHITECTURE.md](../ARCHITECTURE.md).

For user-facing flows (install → UI → analyze → share) mapped to these contracts, see [user-stories.md](user-stories.md). For PR order, agent prompts, and CI gates, see [pr-roadmap.md](pr-roadmap.md). For boundaries vs delegation, see [build-strategy.md](build-strategy.md).

## Roles

| Component | Role | Trust |
|-----------|------|-------|
| **looptap** (Go, local subprocess) | Parse JSONL → SQLite → signals → patterns → emit `tracers.rule/v1` with `source: "template"` | Untrusted input (transcripts); trusted output *shape* |
| **tracers** (Zig, user-facing) | File watcher, SQLite workflow, local UI, authoritative redaction, subprocess orchestration, signing | Trusted edge — secrets never leave unredacted |
| **Modal API** (Python, cloud) | Stateless LLM polish on **already-redacted** evidence; returns enriched cards | Untrusted network; must never receive raw turns |

**Invariant:** Raw transcript bytes never leave the machine. Cloud sees only tracers-redacted excerpts. Share sees only tracers-signed, re-redacted cards.

## Pipeline

```
~/.claude/projects/*.jsonl
        │
        ▼
   looptap run / signal          (parse + detect)
        │
        ▼
   looptap patterns --format json
        │
        ▼
   tracers ingest                 (validate → SQLite `detected`)
        │
   ┌────┴────┐
   │ Local UI │  dismiss / analyze / address / share
   └────┬────┘
        │ Analyze (redacted)
        ▼
   Modal POST /v1/analyze  ──►  enriched card (`source: "llm"`)
        │
        ▼
   tracers merge + `proposed`
        │
        ▼ Share (re-redact + sign)
   POST /v1/inbox
```

---

## Contract 0: Ingestion prerequisite

`looptap patterns` reads **SQLite**, not JSONL directly. tracers must refresh the DB before calling patterns:

```bash
looptap run    --db /path/to/looptap.db
looptap signal --db /path/to/looptap.db
looptap patterns --format json --db /path/to/looptap.db [--min-sessions N]
```

### Security: subprocess invocation

tracers invokes looptap with **`std.process.Child` and an exact argument slice** — no shell (`/bin/sh -c`). Example:

```zig
&[_][]const u8{ "looptap", "patterns", "--format", "json", "--db", db_path }
```

Paths come from tracers config (`~/.tracers/config.toml`), never from unvalidated UI input. Reject paths containing `\0`, newlines, or `..` escapes before exec. Branch names, file paths, and project names from looptap can contain shell metacharacters; parameterized execution eliminates local command injection.

### DB ownership

Pick one owner and document it:

- **Option A:** tracers owns one SQLite file; passes `--db` to every looptap call.
- **Option B:** looptap owns ingestion; tracers read-only on the same path with a file lock.

Do not split writes across two owners without explicit locking.

---

## Contract 1: Local handoff (looptap → tracers)

**Command:** `looptap patterns --format json`

**Output:** Full `tracers.rule/v1` **Bundle** — see [schemas/tracers.rule.v1.json](schemas/tracers.rule.v1.json).

```json
{
  "schema": "tracers.rule/v1",
  "generated_at": "2026-06-20T12:00:00Z",
  "cards": [
    {
      "id": "failure-bash-enoent",
      "pattern": {
        "signal": "failure",
        "tool": "Bash",
        "error_class": "ENOENT",
        "summary": "Bash commands fail with \"No such file or directory\" on paths the agent assumed existed",
        "session_count": 7,
        "example_session_ids": ["9ffb1c2d", "4d308a4c"]
      },
      "evidence": [
        {
          "session_id": "9ffb1c2d",
          "turn_idx": 42,
          "tool_name": "Bash",
          "is_error": true,
          "excerpt": "cd packages/api && npm test\nbash: cd: packages/api: No such file or directory",
          "redactions": 0
        }
      ],
      "rule": {
        "title": "Verify a path exists before using it",
        "snippet": "Before `cd <dir>` or running a command in a subdirectory, confirm the directory exists (e.g. `ls <dir>`); don't assume a path from memory.",
        "rationale": "Bash steps repeatedly fail with ENOENT on directories the agent assumed were present (seen in 7 sessions).",
        "target": "AGENTS.md",
        "confidence": "medium",
        "source": "template"
      },
      "signature": ""
    }
  ]
}
```

### Implementer notes

| Field | Rule |
|-------|------|
| `id` | Deterministic slug: `signal-tool-error_class` → `failure-bash-enoent`. Primary key for tracers upserts. |
| `signature` | Always `""` from looptap. tracers fills at share time. |
| `generated_at` | RFC3339 UTC. Required on every bundle. |
| Empty `cards` | Valid — nothing crossed `--min-sessions`. Not an error. |

### tracers ingest

1. Parse JSON; reject if `schema != "tracers.rule/v1"`.
2. Validate against [tracers.rule.v1.json](schemas/tracers.rule.v1.json).
3. Re-redact every `evidence[].excerpt` with the authoritative Zig redactor; update `redactions`.
4. **Write scrubbed bytes only** — persist `card_json` to SQLite *after* step 3. The local `insights` database must never hold raw secrets (accidental `insights.db` leak or UI XSS should have zero blast radius). Raw secrets exist only in source `.jsonl` files.
5. Upsert into SQLite: new cards → `detected`; do not overwrite `addressed` / `ignored` on rescan unless user opts in.

### Redaction layers

| Stage | Who | Purpose |
|-------|-----|---------|
| Pre-pass | looptap (`internal/rule/redact.go`) | Safe local pipes (`patterns \| jq`) |
| Authoritative (ingest) | tracers (`redact.zig`) | **Before `insights.card_json` is written** |
| Authoritative (egress) | tracers (`redact.zig`) | Immediately before HTTP and signing |

tracers is the source of truth. looptap's pre-pass is best-effort; missed secrets must be caught at ingest, not deferred to share time.

---

## Contract 2: State machine (tracers SQLite)

The product moat. Store the **whole card** as canonical JSON plus workflow metadata.

```sql
CREATE TABLE insights (
    id              TEXT PRIMARY KEY,     -- card.id from tracers.rule/v1
    state           TEXT NOT NULL CHECK (state IN (
                        'detected', 'analyzing', 'proposed',
                        'addressed', 'ignored', 'failed'
                    )),
    card_json       TEXT NOT NULL,        -- full Card object
    source          TEXT NOT NULL,        -- 'template' | 'llm' (from card.rule.source)
    session_count   INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,                 -- when state = 'failed'
    created_at      TEXT NOT NULL,        -- RFC3339 UTC
    updated_at      TEXT NOT NULL,
    analyzed_at     TEXT,
    addressed_at    TEXT
);

CREATE INDEX insights_state_idx ON insights(state);
CREATE INDEX insights_updated_idx ON insights(updated_at);
```

### State transitions

```
detected ──[Analyze]──► analyzing ──[Modal OK]──► proposed
                              └──[Modal err]──► failed ──[Retry]──► analyzing
detected ──[Dismiss]──► ignored
proposed ──[Save to CLAUDE.md]──► addressed
proposed ──[Dismiss]──► ignored
detected ──[Rescan, not addressed/ignored]──► updated in place
```

**UI transport (Phase 2):** SSE or HTMX polling while `state = analyzing`. WebSockets only if bidirectional chat is needed later.

**Save to CLAUDE.md:** Append `rule.snippet` to the path named in `rule.target` (`AGENTS.md` or `CLAUDE.md`).

---

## Contract 3: Cloud analysis (tracers → Modal → tracers)

### Security boundary

- tracers runs authoritative redaction on every excerpt **before** HTTP.
- Modal never receives raw turn content, full session IDs, or filesystem paths unless explicitly allowlisted in the request schema.
- **Evidence cap:** tracers sends at most **5** redacted evidence turns per analyze request (`redacted_evidence.maxItems` in schema), even if looptap attached more or a cluster spans huge sessions. Bounds Modal payload size and protects against accidental DoS.
- Modal holds LLM API keys; tracers holds transcripts. Keys never enter looptap's subprocess environment.

### Request — `POST /v1/analyze`

Schema: [schemas/tracers.analyze.v1.request.json](schemas/tracers.analyze.v1.request.json)

```json
{
  "api_version": "tracers.analyze/v1",
  "request_id": "req-999",
  "card_id": "failure-bash-enoent",
  "pattern": {
    "signal": "failure",
    "tool": "Bash",
    "error_class": "ENOENT",
    "summary": "Bash commands fail with ENOENT on assumed paths",
    "session_count": 7
  },
  "redacted_evidence": [
    {
      "tool_name": "Bash",
      "is_error": true,
      "excerpt": "bash: cd: packages/api: No such file or directory",
      "redactions": 1
    }
  ],
  "template_rule": {
    "title": "Verify a path exists before using it",
    "snippet": "Before `cd <dir>`...",
    "target": "AGENTS.md",
    "confidence": "medium"
  }
}
```

**Why `template_rule`:** Modal polishes wording; it does not invent patterns. On LLM failure, tracers keeps the template card and sets `state = failed`.

### Response

Schema: [schemas/tracers.analyze.v1.response.json](schemas/tracers.analyze.v1.response.json)

Modal returns one **complete Card**. tracers merges locally:

```json
{
  "api_version": "tracers.analyze/v1",
  "request_id": "req-999",
  "card": {
    "id": "failure-bash-enoent",
    "pattern": { "... unchanged or validated refinement ..." },
    "evidence": [ "... same redacted evidence as request ..." ],
    "rule": {
      "title": "Verify paths before cd",
      "snippet": "Before `cd <dir>`, run `ls <dir>` to confirm the directory exists.",
      "rationale": "Agent repeatedly assumes directory layout, causing ENOENT retry loops.",
      "target": "AGENTS.md",
      "confidence": "high",
      "source": "llm"
    },
    "signature": ""
  }
}
```

### Merge rules (tracers-side)

| Field | On analyze success |
|-------|-------------------|
| `id` | Must equal request `card_id` |
| `pattern` | Keep local unless Modal returns validated refinements only |
| `evidence` | **Never** replace with Modal output — cloud must not widen redacted content |
| `rule` | Replace if `source == "llm"` and schema-valid |
| `signature` | Always `""` until share |

Modal validates with Pydantic/Instructor. Reject responses where evidence bytes differ from the request fingerprint, where `source != "llm"`, or where `confidence` ∉ `{high, medium, low}`.

---

## Contract 4: Share (tracers → share server)

### Security boundary

Sign **canonical JSON** of the card plus **`expires_at`** (RFC3339 UTC) using JCS (RFC 8785). Re-redact immediately before signing. Embed expiry in the signed payload — not only in the hosted viewer TTL — so a copy-pasted artifact into Slack or a PR comment still fails verification once cryptographically expired.

### Request — `POST /v1/inbox`

Schema: [schemas/tracers.share.v1.request.json](schemas/tracers.share.v1.request.json)

```json
{
  "api_version": "tracers.share/v1",
  "card": {
    "id": "failure-bash-enoent",
    "pattern": { "..." },
    "evidence": [ "..." ],
    "rule": { "source": "llm", "..." },
    "signature": ""
  },
  "attestation": {
    "signer_public_key": "ed25519-hex",
    "algorithm": "ed25519",
    "expires_at": "2026-06-27T12:00:00Z",
    "payload_hash": "sha256-hex-of-canonical-{card,expires_at}-bytes",
    "signature": "base64"
  }
}
```

Share server: canonicalize `{ card, expires_at }` → SHA-256 → ed25519 verify → reject if `now > expires_at` → store. Private key never leaves the device.

---

## Phase plan

### Phase 1 — Hybrid scaffold (now)

**tracers:** Embed or pin `looptap` binary. `execve` wrapper: `run` → `signal` → `patterns --format json`. Parse bundle → HTML list. "Analyze" → Modal → display snippet.

**Modal:** `POST /v1/analyze` with Pydantic validation. No clustering, no raw transcript upload.

**looptap:** Stable `patterns --format json` spine. tracers depends on `patterns`, not `advise`.

**Exit criteria:** One failure pattern flows detect → analyze → display with no raw secrets in Modal logs.

### Phase 2 — State machine moat

**tracers:** SQLite `insights`, dismiss / analyze / save actions, SSE or HTMX for `analyzing → proposed`, idempotent rescans.

**Modal:** Structured output enforcement, rate limits, `request_id` dedup.

**Exit criteria:** User manages a backlog; cloud outage leaves template cards usable in `detected`.

### Phase 3 — Consolidation (if justified)

**Do not** lead with streaming raw transcripts to Modal — privacy regression and cost spike.

| Keep local | OK in cloud |
|------------|-------------|
| JSONL parse, incremental scan | LLM polish |
| Deterministic signals (failure, loop, …) | Cross-session clustering *if* embeddings are needed |
| `--min-sessions` gating | CLAUDE.md quality review |
| Offline / no-API-key mode | — |

**Preferred path:** Port looptap's parser + signal + patterns into tracers (Zig); retire the Go **binary**, not the **algorithms**. Keep deterministic detection local; cloud stays stateless LLM polish.

---

## Open decisions

1. ~~**Single SQLite file**~~ — **Resolved:** two files. `looptap.db` (engine, Go-only) + `insights.db` (workflow, Zig-only). Handoff is stdout JSON, not shared SQLite. See [build-strategy.md](build-strategy.md).
2. **Target file default** — looptap templates default to `AGENTS.md`; UI "Save" must respect `rule.target`.
3. **Canonical JSON for signing** — adopt JCS (RFC 8785) with shared test vectors between tracers and share server. Sign bytes cover `{ card, expires_at }`.
4. **API versioning** — `tracers.analyze/v1` and `tracers.share/v1` are separate from `tracers.rule/v1` so HTTP envelopes can evolve without breaking the card record.

---

## Related files in this repo

| Path | What |
|------|------|
| `internal/rule/types.go` | `tracers.rule/v1` Go types + `MarshalBundle` |
| `internal/rule/types_test.go` | Golden round-trip against tracers spec |
| `internal/rule/redact.go` | Pre-pass redactor (non-authoritative) |
| `cmd/patterns.go` | `--format json` bundle emitter |
| `testdata/contracts/` | Golden fixtures — looptap owns, tracers copies |
| `deploy/app.py` | Modal hosting (evolve toward `/v1/analyze`) |
| `docs/pr-roadmap.md` | **Source of truth** — PR order, prompts, CI matrix |
| `docs/hybrid-architecture.md` | Cross-system contracts and phase plan |
| `docs/user-stories.md` | User stories → technical contracts + security checklist |
| `docs/tracers-scaffold.md` | tracers file-level map |
| `docs/build-strategy.md` | Boundaries vs delegation, two-DB model |
