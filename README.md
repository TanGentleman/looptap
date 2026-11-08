# looptap

Tweak that agent loop. Reads your coding agent's transcripts, flags the rough patches — correction loops, dead ends, tool-call death spirals — and dumps it to SQLite for datasette or ready-to-paste CLAUDE.md rules. Everything runs offline; no API key.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/install.sh | bash
```

Or from source (needs cgo for SQLite): `go install github.com/TanGentleman/looptap@latest`.

## Try it

```bash
looptap run && looptap info
```

That parses every Claude Code and Codex transcript under `~/.claude/projects/` and `~/.codex/sessions/` (or `$CLAUDE_CONFIG_DIR`, `$CODEX_HOME`; pass a directory to `run` to read somewhere else), fires the seven detectors, and prints what it found. Browse the database with [datasette](https://datasette.io/):

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

That's the tour. `looptap --help` lists the rest (`parse`, `patterns`, `query`, `seed-contract-fixture`, `signal`, `version`); the why-and-how lives in [ARCHITECTURE.md](ARCHITECTURE.md).

No transcripts handy? The repo ships fixtures:

```bash
git clone https://github.com/TanGentleman/looptap && cd looptap
go run . run testdata/claude_code testdata/codex --db /tmp/demo.db
go run . query --db /tmp/demo.db --signal failure --format paths
# testdata/codex/rollout-2026-11-01T10-00-00-0199a000-0000-7000-8000-000000000001.jsonl
```

Pipe rough sessions to whatever tool wants them next:

```bash
looptap query --signal failure --signal misalignment --format paths | xargs tar -czf bad-runs.tgz
```

Or go one rung up — cluster the failures that *keep* happening into ready-to-paste rules (no API key needed):

```bash
looptap patterns                  # human-readable
looptap patterns --format json    # a tracers.rule/v1 bundle for downstream tools
```

## More

- [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, prompts, every knob.
- [docs/contracts.md](docs/contracts.md) — what looptap puts on the wire (`tracers.rule/v1`). Cross-system plan lives in tracers.
- [`scripts/`](scripts) — install and release tooling.
