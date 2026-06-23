# looptap contracts

What looptap puts on the wire. The cross-system narrative — state machine, Modal,
share, signing — lives in tracers `docs/architecture.md`; this file describes only
the producer side.

looptap emits `tracers.rule/v1` on stdout via `looptap patterns --format json`.
tracers consumes the bundle, re-redacts authoritatively, and drives the workflow.

## Argv stability (Contract 0 — producer side)

```
looptap run --db <path>
looptap signal --db <path>
looptap patterns --format json --db <path> [--min-sessions N]
```

Consumers must invoke via `std.process.Child` / `exec` with an argv slice — no
shell. Path-validation rules are the consumer's responsibility and are not
restated here.

## Wire shape (Contract 1 — producer side)

One trimmed `tracers.rule/v1` Bundle (full schema:
[docs/schemas/tracers.rule.v1.json](schemas/tracers.rule.v1.json); golden
fixture: [testdata/contracts/tracers.rule.v1.golden-bundle.json](../testdata/contracts/tracers.rule.v1.golden-bundle.json)):

```json
{
  "schema": "tracers.rule/v1",
  "generated_at": "2026-06-20T12:00:00Z",
  "gate_min_sessions": 5,
  "cards": [
    {
      "id": "failure-bash-enoent",
      "pattern": {
        "signal": "failure",
        "tool": "Bash",
        "error_class": "ENOENT",
        "session_count": 7,
        "example_session_ids": ["9ffb1c2d", "4d308a4c"]
      },
      "evidence": [
        {
          "session_id": "9ffb1c2d",
          "turn_idx": 42,
          "tool_name": "Bash",
          "is_error": true,
          "excerpt": "bash: cd: packages/api: No such file or directory",
          "redactions": 0
        }
      ],
      "rule": { "target": "AGENTS.md", "confidence": "medium", "source": "template" },
      "signature": ""
    }
  ]
}
```

Empty cards: `[]` is valid — nothing crossed `--min-sessions`, not an error.

## Field invariants looptap guarantees

- `id` is a `signal-tool-error_class` slug (e.g. `failure-bash-enoent`).
- `signature` is always `""` from looptap; tracers fills it at share time.
- `generated_at` is RFC3339 UTC.
- `gate_min_sessions` mirrors the `--min-sessions` flag — the consumer cannot drift from the producer.
- `rule.source` is `"template"` from looptap; the LLM path stamps `"llm"` downstream.

## Redaction layering

`internal/rule/redact.go` is a best-effort pre-pass for safe local pipes (`looptap patterns --format json | jq`). It is explicitly not authoritative: tracers re-redacts every excerpt at ingest, and again at share. Do not grow the pre-pass into a redaction engine.

## Outstanding looptap-side work

Modal `POST /v1/analyze` lives in `deploy/` — see tracers' cross-system roadmap for the consumer side.

## Pointers

| Where | What |
|-------|------|
| tracers `docs/architecture.md` | cross-system narrative, state machine, share/sign |
| tracers `docs/rule-with-evidence.md` | contract spec (signing, JCS) |
| `docs/schemas/` | JSON Schemas (canonical) |
| `testdata/contracts/` | golden fixtures (looptap owns; tracers copies) |
| `ARCHITECTURE.md` | looptap internals (parsers, detectors, SQLite, prompts) |
