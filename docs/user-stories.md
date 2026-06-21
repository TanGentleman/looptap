# User stories and contracts

End-to-end stories for the hybrid stack — what users see, what stays opaque, and
which technical contracts enable each flow. Cross-system roles and phase plan live
in [hybrid-architecture.md](hybrid-architecture.md); JSON Schema stubs under
[schemas/](schemas/).

## Division of labor

| Component | Role | User-facing? |
|-----------|------|--------------|
| **looptap** | Silent engine room — parse JSONL, detect signals, cluster patterns, emit `tracers.rule/v1` | No |
| **tracers** | Secure edge and UX — state machine, authoritative redaction, signing, HTML + Zig UI | Yes |
| **Modal** | Stateless brain — LLM polish on already-redacted evidence only | No |

**Invariant:** Raw transcript bytes never leave the machine. The only place unredacted
secrets may exist is the original `.jsonl` files under `~/.claude/projects/`.

---

## Story 1 — One command to install, UI opens

**As a** developer who uses Claude Code  
**I want** a single install command that puts an app on my machine and opens a dashboard  
**So that** I can see what's going wrong in my sessions without reading JSONL or running CLI flags

### What the user sees

```bash
curl -fsSL https://tracers.dev/install | sh && tracers open
```

Browser opens `http://127.0.0.1:3000`. Overview cards, signal breakdown, and a list
of **insights** (recurring failure patterns): e.g. "Bash keeps failing with missing
directories."

### What stays opaque

- looptap subprocess chain (`run` → `signal` → `patterns`)
- Engine vs workflow SQLite files
- Loopback bearer token
- Whether data is live or still scanning

### Technical contracts

| Layer | Contract |
|-------|----------|
| **Install** | One static tracers binary + pinned looptap; `tracers open` starts core + UI |
| **Core daemon** | `tracers serve --addr 127.0.0.1:8787` — loopback-only, bearer auth |
| **UI** | HTML + Zig (spider + HTMX), separate binary like tracers-web; polls core |
| **Bootstrap scan** | Contract 0 in [hybrid-architecture.md](hybrid-architecture.md): fixed-argv looptap chain |
| **Ingest** | Parse `tracers.rule/v1` Bundle → **redact.zig on every excerpt** → upsert `insights` as `detected` |
| **UI read API** | `GET /api/insights?state=active` → display DTOs: `{ id, title, summary, session_count, state_label, confidence }` |

### Security: subprocess invocation

tracers invokes looptap with **`std.process.Child` and an exact argument slice** —
never `/bin/sh -c`. Example:

```zig
&[_][]const u8{ "looptap", "patterns", "--format", "json", "--db", db_path }
```

Paths and flags come from tracers config (`~/.tracers/config.toml`), never from
unvalidated UI input. Reject paths containing `\0`, newlines, or `..` before exec.
Branch names, file paths, and project names from looptap output can contain shell
metacharacters; parameterized execution eliminates local command injection.

---

## Story 2 — See insights and pipeline stage

**As a** user reviewing my agent habits  
**I want** to see recurring problems and whether I've already dealt with them  
**So that** I focus on new issues and don't re-litigate old ones

### What the user sees

| User-facing bucket | Internal states (opaque) |
|--------------------|--------------------------|
| **New** | `detected` |
| **Analyzing…** | `analyzing` |
| **Ready to apply** | `proposed` |
| **Saved** | `addressed` |
| **Dismissed** | `ignored` |
| **Couldn't analyze** | `failed` (retry offered) |

Each row: title, "seen in N sessions," confidence badge, expandable redacted evidence.

### Technical contracts

| Layer | Contract |
|-------|----------|
| **Canonical record** | Full `tracers.rule/v1` Card in `insights.card_json` — see [schemas/tracers.rule.v1.json](schemas/tracers.rule.v1.json) |
| **Workflow DB** | Contract 2 `insights` table in [hybrid-architecture.md](hybrid-architecture.md) |
| **Rescan merge** | Update `detected` in place; never overwrite `addressed` / `ignored` unless user opts in |
| **List API** | `GET /api/insights?bucket=new\|ready\|done` — server maps internal states → buckets |
| **Detail API** | `GET /api/insights/:id` → card (evidence already scrubbed at ingest) |

### Security: redaction before SQLite write

**Apply `redact.zig` before persisting `card_json`.** Ingest is not "store raw, redact
on share." The local `insights` database must contain only scrubbed excerpts so that:

- Accidental leakage of `insights.db` exposes no secrets
- A UI XSS bug cannot exfiltrate raw API keys from stored evidence

looptap's pre-pass (`internal/rule/redact.go`) is defense in depth for local pipes;
tracers re-redacts authoritatively at ingest, before any SQLite write, and again
immediately before HTTP or signing.

The ≥5-session gate is invisible to the user: looptap `--min-sessions` and tracers
ingest only surface cards that cleared the gate.

---

## Story 3 — Analyze further (cloud polish)

**As a** user who trusts the pattern but wants sharper wording  
**I want** to click **Analyze** on an insight  
**So that** I get a clearer, paste-ready rule without sending raw transcripts to the cloud

### What the user sees

**Analyze** on a New insight → **Analyzing…** → **Ready to apply** with improved
title/snippet. On failure, template wording remains usable with "Retry?"

### Technical contracts

| Step | Contract |
|------|----------|
| **UI action** | `POST /api/insights/:id/analyze` |
| **State** | `detected \| failed` → `analyzing` → `proposed` |
| **Pre-flight** | Re-redact excerpts; cap evidence (see below); build analyze request |
| **Cloud request** | `POST /v1/analyze` — [schemas/tracers.analyze.v1.request.json](schemas/tracers.analyze.v1.request.json) |
| **Cloud response** | [schemas/tracers.analyze.v1.response.json](schemas/tracers.analyze.v1.response.json) |
| **Merge** | Keep local `pattern` + `evidence`; replace `rule` only if valid; reject evidence drift |
| **UI transport** | HTMX poll or SSE until `state != analyzing` |

### Security: cloud request shape and evidence cap

The analyze request **strips `session_id`** from evidence sent to Modal — cloud
never receives raw turn identity or filesystem paths.

**Hard cap: max 5 evidence turns** per analyze request (`redacted_evidence.maxItems`
in schema). looptap may attach up to 3 examples per card; tracers must truncate
before HTTP even if a cluster is huge. This bounds Modal payload size and protects
against accidental DoS from pathological loops (e.g. 10,000-turn sessions clustered
into one pattern).

Modal holds LLM API keys; tracers holds transcripts. Keys never enter looptap's
subprocess environment.

---

## Story 4 — Save remediation to my project

**As a** user who agrees with a ready insight  
**I want** to click **Save to AGENTS.md** (or CLAUDE.md)  
**So that** my agent stops repeating the same mistake

### Technical contracts

| Layer | Contract |
|-------|----------|
| **UI action** | `POST /api/insights/:id/address` — optional `{ "target": "AGENTS.md" }`; default from `card.rule.target` |
| **File write** | Append `card.rule.snippet` to resolved path |
| **State** | `proposed` → `addressed`; set `addressed_at` |

---

## Story 5 — Share a full transcript

**As a** user debugging a bad session  
**I want** to share one flagged session with a teammate  
**So that** they can read the redacted transcript via a one-time link

### What the user sees

**Share** on a flagged session → copy link `http://127.0.0.1:8787/s/<token>`. Viewer
shows redacted transcript; "N secrets stripped" badge.

### Technical contracts (existing tracers)

| Layer | Contract |
|-------|----------|
| **Mint** | `POST /share/<session_id>` on `tracers serve` |
| **Redaction** | `redact.zig` on full transcript before store |
| **View** | `GET /s/<token>` → HTML viewer |
| **Audit** | `~/.tracers/audit.log` — no token or body bytes |

This path shares **transcripts**, not rule cards. Use Story 6 for insight sharing.

---

## Story 6 — Share an insight (pattern + evidence + remediation)

**As a** user who fixed a recurring failure  
**I want** to share the insight — not the whole transcript — with my team  
**So that** others can verify and adopt the same rule

### What the user sees

**Share insight** on Ready or Saved → link to compact viewer: pattern summary,
redacted evidence, snippet, optional "Signed by @you" badge.

### Technical contracts

| Layer | Contract |
|-------|----------|
| **UI action** | `POST /api/insights/:id/share` |
| **Pre-share** | Re-redact excerpts; canonicalize (JCS RFC 8785); sign with Ed25519 (`identity.zig`) |
| **Hosted share** | `POST /v1/inbox` — [schemas/tracers.share.v1.request.json](schemas/tracers.share.v1.request.json) |
| **Local mint (Phase 1)** | Reuse share store pattern; body = signed envelope or canonical card JSON |

### Security: signature and cryptographic expiry

Ed25519 over canonical JSON (JCS) is the trust anchor. The signed envelope includes
**`expires_at`** (RFC3339 UTC) in the attested payload — not only in the hosted
viewer TTL. If someone copy-pastes the signed JSON into Slack or a GitHub PR, any
client verifying the signature can also reject cryptographically expired artifacts.

Canonical sign bytes cover `{ card, expires_at }` (card with `signature: ""`).
See attestation fields in the share schema.

---

## Story 7 — Dismiss noise

**As a** user who doesn't want to act on a pattern  
**I want** to dismiss an insight  
**So that** it stops cluttering New but doesn't resurrect on every rescan

### Technical contracts

| Layer | Contract |
|-------|----------|
| **UI action** | `POST /api/insights/:id/dismiss` |
| **State** | `detected \| proposed` → `ignored` |
| **Rescan** | Ingest skips `ignored` / `addressed` by stable `card.id` slug |

---

## Story 8 — Background refresh

**As a** user who codes all day  
**I want** new sessions to show up without manual refresh  
**So that** the dashboard stays current

### What the user sees

"Updated 2 min ago — 1 new insight." No CLI.

### Technical contracts

| Layer | Contract |
|-------|----------|
| **Watcher** | Mtime/size on `~/.claude/projects/**/*.jsonl` |
| **Incremental engine** | Fixed-argv: `looptap run` → `looptap signal` → `looptap patterns --format json` |
| **Debounce** | Coalesce events; cap scan frequency |
| **Ingest** | Same as Story 1 — redact before SQLite write |
| **UI** | HTMX poll or SSE `insights_updated` |

---

## Story 9 — Receive a shared insight

**As a** teammate opening a shared link  
**I want** to read the pattern, evidence, and suggested rule  
**So that** I can paste it into our repo without trusting an unverified paste

### Technical contracts

| Layer | Contract |
|-------|----------|
| **Payload** | `tracers.rule/v1` Card (or single-card bundle) |
| **Verify** | Ed25519 over canonical bytes; check `expires_at` |
| **Privacy** | No raw paths, full session IDs, or secrets in payload |

---

## UI action surface (minimal)

Expose only on each insight row:

| Action | When | Under the hood |
|--------|------|----------------|
| **Analyze** | New, Couldn't analyze | Modal `tracers.analyze/v1` |
| **Save** | Ready to apply | Append `rule.snippet` |
| **Share insight** | Ready, Saved | Signed `tracers.share/v1` |
| **Dismiss** | New, Ready | `ignored` state |

Separate **Share session** on the flagged-sessions panel (Story 5).

---

## Contract map

```
~/.claude/projects/*.jsonl
        │
        ▼  (fixed argv — no shell)
   looptap run → signal → patterns --format json
        │
        ▼  tracers.rule/v1 Bundle
   redact.zig (before SQLite write)
        │
        ▼
   insights SQLite + state machine
        │
   ┌────┴────────────────────────────┐
   │ HTML + Zig UI                    │
   │  Analyze ──► Modal /v1/analyze   │  (≤5 evidence turns, no session_id)
   │  Share insight ──► JCS + Ed25519 │  (expires_at in signed envelope)
   │  Share session ──► /s/<token>    │  (existing transcript path)
   └──────────────────────────────────┘
```

| User action | Primary schema / API |
|-------------|----------------------|
| See insights | `tracers.rule/v1` ingest |
| Analyze | `tracers.analyze/v1` request/response |
| Save rule | `card.rule.target` + `snippet` |
| Share session | `POST /share/:id` → `/s/:token` |
| Share insight | `tracers.share/v1` + attestation |

---

## Security refinements (build checklist)

Before implementation, treat these as non-negotiable:

1. **Redaction at ingest** — `redact.zig` runs before `insights.card_json` is written.
   Raw secrets exist only in source `.jsonl` files.

2. **Cloud evidence cap** — Max 5 turns in `redacted_evidence`; no `session_id` in
   analyze request. Modal validates payload size and schema.

3. **No shell for looptap** — `std.process.Child` with exact `argv`; paths from config
   only; reject `\0`, newlines, `..`.

4. **Signed expiry** — `expires_at` included in canonical sign bytes for share
   artifacts so verification works outside the hosted viewer TTL.

See [hybrid-architecture.md](hybrid-architecture.md) for the full contract set and
phase plan. **tracers implementers:** start with [tracers-scaffold.md](tracers-scaffold.md).
