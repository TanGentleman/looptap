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

That's the tour. `looptap --help` lists the rest (`advise`, `aggregate`, `analyze`, `html`, `parse`, `query`, `signal`, `version`); the why-and-how lives in [ARCHITECTURE.md](ARCHITECTURE.md).

## Zoom out: the whole fleet

One person's transcripts tell one story. Pool many people's and the patterns get loud. Tag sessions as you ingest them, then roll the signals up across everyone:

```bash
looptap run --user alice --team payments    # each engineer ingests their own
looptap aggregate                            # what keeps going wrong, fleet-wide
looptap aggregate --team payments --min-confidence 0.7
looptap aggregate --advise                   # turn the rollup into CLAUDE.md rules (LLM)
```

`aggregate` reports the failing tools, the patterns that recur *across users*, and the teams carrying the most signal per session — all in plain SQL, no API key. See it end to end with `scripts/fleet-demo.sh` (a synthetic five-engineer, two-team fleet).

Pipe rough sessions to whatever tool wants them next:

```bash
looptap query --signal failure --signal misalignment --format paths | xargs tar -czf bad-runs.tgz
```

## More

- [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, prompts, every knob.
- [`deploy/`](deploy) — host `looptap analyze` and the repo-analysis API on Modal.
- [`scripts/`](scripts) — install, uninstall, deploy, cut a release.
- [`.github/workflows/html-report.yml`](.github/workflows/html-report.yml) — branch reports from CI via opencode.
