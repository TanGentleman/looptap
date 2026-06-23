# Cross-repo contract fixtures

looptap owns these files. tracers copies them into `test/fixtures/looptap/` and pins the source commit SHA in a header comment.

**Do not edit field names here without bumping `tracers.rule/v1` and updating both repos' CI.**

| File | Purpose |
|------|---------|
| `tracers.rule.v1.golden-card.json` | Canonical card — matches tracers `rule-with-evidence.md` |
| `tracers.rule.v1.golden-bundle.json` | Full bundle envelope (`schema`, `generated_at`, `gate_min_sessions`, `cards[]`) |
| `tracers.rule.v1.empty-bundle.json` | Valid zero-card bundle — `cards: []`, never `null` |
| `tracers.rule.v1.leaky-bundle.json` | Card with fake API key in excerpt — redaction CI |
| `tracers.analyze.v1.request.golden.json` | Cloud analyze payload (no `session_id` in evidence) |
| `tracers.analyze.v1.response.golden.json` | Modal response envelope + enriched card |

Schemas: [docs/schemas/](../../docs/schemas/). Every fixture is validated against its schema in CI by `internal/rule/contract_test.go`.
