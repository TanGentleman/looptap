# Hybrid Architecture: looptap + tracers + Modal

Cross-system contract for the pipeline that moves a transcript from a raw JSONL file to a signed, shareable insight. looptap stays the **deterministic engine**; tracers is the **trusted edge** (UI, redaction, workflow, signing); Modal is the **stateless LLM polish** layer.

For looptap-only internals (parsers, detectors, SQLite schema), see [ARCHITECTURE.md](../ARCHITECTURE.md).

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

tracers invokes looptap with a **fixed argv slice** (`execve` style). No shell (`/bin/sh -c`). Paths come from tracers config (`~/.tracers/config.toml`), never from unvalidated UI input. Reject paths containing `\0`, newlines, or `..` escapes before exec.

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
4. Upsert into SQLite: new cards → `detected`; do not overwrite `addressed` / `ignored` on rescan unless user opts in.

### Redaction layers

| Stage | Who | Purpose |
|-------|-----|---------|
| Pre-pass | looptap (`internal/rule/redact.go`) | Safe local pipes (`patterns \| jq`) |
| Authoritative | tracers (`redact.zig`) | Before SQLite cloud fields, HTTP, and signing |

tracers is the source of truth. looptap's pre-pass is best-effort sync; missed secrets are caught downstream.

---

## Contract 2: State machine (tracers SQLite)

The product moat. Store the **whole card** as canonical JSON plus workflow metadata.

```sql
CREATE TABLE findings (
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

CREATE INDEX findings_state_idx ON findings(state);
CREATE INDEX findings_updated_idx ON findings(updated_at);
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

Sign **canonical JSON** of the card (JCS / RFC 8785, or a documented stable key order shared with the share server). Re-redact immediately before signing.

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
    "payload_hash": "sha256-hex-of-canonical-card-bytes",
    "signature": "base64"
  }
}
```

Share server: canonicalize → SHA-256 → ed25519 verify → store. Private key never leaves the device.

---

## Phase plan

### Phase 1 — Hybrid scaffold (now)

**tracers:** Embed or pin `looptap` binary. `execve` wrapper: `run` → `signal` → `patterns --format json`. Parse bundle → HTML list. "Analyze" → Modal → display snippet.

**Modal:** `POST /v1/analyze` with Pydantic validation. No clustering, no raw transcript upload.

**looptap:** Stable `patterns --format json` spine. tracers depends on `patterns`, not `advise`.

**Exit criteria:** One failure pattern flows detect → analyze → display with no raw secrets in Modal logs.

### Phase 2 — State machine moat

**tracers:** SQLite `findings`, dismiss / analyze / save actions, SSE or HTMX for `analyzing → proposed`, idempotent rescans.

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

1. **Single SQLite file** — tracers-owned with looptap `--db`, or looptap-owned with tracers read-only?
2. **Target file default** — looptap templates default to `AGENTS.md`; UI "Save" must respect `rule.target`.
3. **Canonical JSON for signing** — adopt JCS (RFC 8785) with shared test vectors between tracers and share server.
4. **API versioning** — `tracers.analyze/v1` and `tracers.share/v1` are separate from `tracers.rule/v1` so HTTP envelopes can evolve without breaking the card record.

---

## Related files in this repo

| Path | What |
|------|------|
| `internal/rule/types.go` | `tracers.rule/v1` Go types + `MarshalBundle` |
| `internal/rule/types_test.go` | Golden round-trip against tracers spec |
| `internal/rule/redact.go` | Pre-pass redactor (non-authoritative) |
| `cmd/patterns.go` | `--format json` bundle emitter |
| `deploy/app.py` | Modal hosting (evolve toward `/v1/analyze`) |
