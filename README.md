# looptap

Tweak that agent loop. Reads your coding agent's transcripts, flags the rough patches — correction loops, dead ends, tool-call death spirals — and dumps it to SQLite for datasette or LLM-driven CLAUDE.md suggestions.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/install.sh | bash
```

## Try it

```bash
looptap run && looptap info
```

That parses every transcript under `~/.claude/projects/`, fires the seven detectors, and prints what it found. Browse the database with [datasette](https://datasette.io/):

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

That's the tour. `looptap --help` lists the rest (`advise`, `analyze`, `html`, `parse`, `patterns`, `query`, `seed-contract-fixture`, `signal`, `version`); the why-and-how lives in [ARCHITECTURE.md](ARCHITECTURE.md).

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
- [`deploy/`](deploy), [`scripts/`](scripts) — Modal hosting and install/release tooling.
