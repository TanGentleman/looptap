# looptap

Reads Claude Code and Codex transcripts from disk, flags failures, loops and
corrections with seven offline detectors, and writes them to SQLite. No network or
API key.

```sh
git clone https://github.com/TanGentleman/looptap && cd looptap
go build -o looptap .
./looptap run testdata/claude_code testdata/codex --db /tmp/demo.db
```

Needs Go 1.26 or newer and a C compiler (the SQLite driver uses cgo). Details:
[IMPLEMENTATION.md](IMPLEMENTATION.md), [ROADMAP.md](ROADMAP.md).
