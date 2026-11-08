# looptap roadmap

looptap reads Claude Code (and soon Codex) transcripts from disk, flags rough patches
(correction loops, failing tools, repeated tool calls) with seven offline detectors, and
writes sessions, turns and signals to SQLite. `patterns` then turns failures that repeat
across sessions into CLAUDE.md/AGENTS.md rules. For people who use coding agents daily and
want to see where the agent keeps tripping.

## Status (upstream main d8ab84b, checked 2026-11-08)

- Builds and passes on Go 1.24.5: `go vet ./...`, `go test -race ./...` (8 packages),
  `scripts/smoke-db.sh`. Same on Go 1.26.10 after the bumps on `-1`.
- `scripts/install.sh` installs v0.7.0 (2026-04-30). That release has no `patterns` or
  `seed-contract-fixture`, so the README's `looptap patterns` example fails for anyone who
  installed it. main needs a new tag.
- Go 1.24 is out of support (go.dev lists 1.27.3 and 1.26.10). Dependencies were 4-25
  minor versions behind (`go get -u ./...`); genai 1.77 needs grpc 1.85, which needs Go 1.26.
- Codex: `internal/parser/codex.go` is a stub that errors, so every file under
  `~/.codex/sessions` prints `skip ... not yet implemented`.
- `looptap parse <dir>` only accepts files whose path contains `.claude/projects/`:
  `looptap parse testdata/claude_code` finds 0 files.
- Claude Code: only `~/.claude/projects` is scanned; `CLAUDE_CONFIG_DIR` and
  `~/.config/claude/projects` are ignored.
- Dead links: `https://github.com/TanGentleman/tracers` (404 without auth; the contract
  docs depend on it), schema `$id`s on `looptap.dev` (domain does not resolve; ids only).
- Open upstream PRs: #59 dependabot (applied on `-1`), #8 (dirty, Gemini JSON parsing),
  #55-#58 and #60-#62 (Cursor drafts: `start.sh` injection demos and `looptap env`).

## Signal vs noise

Keep: `parse` -> `signal` (`run` does both) -> `query` / `patterns`, plus `info`. The
parser and detector interfaces, SQLite schema, `metadata.json` for datasette.

Cut or fold:
- `html` (asks an agent to write a branch report; unrelated to transcripts),
  `.github/workflows/html-report.yml`, the opencode configs.
- `deploy/` (Modal app), `scripts/setup.sh`, `scripts/deploy.sh`, `example.env`.
- `analyze` (Gemini review of a CLAUDE.md file; does not read transcripts).
- Later, maybe: `advise` (Gemini) now that `patterns` writes rules with no API key; it is
  the only user of genai and most of `go.sum`.
- `seed-contract-fixture`, `docs/schemas`, `testdata/contracts`: only matter to tracers.

## Stack

- `looptap-1` (done): import, Go 1.26 + current deps, actions bump (#59), this file.
- `looptap-2`: `parse <path>` sniffs file contents; scan `CLAUDE_CONFIG_DIR` and
  `~/.config/claude/projects`; skip Claude Code meta and sidechain lines; fixtures.
- `looptap-3`: Codex rollout parser (`~/.codex/sessions`, `archived_sessions`) with fixtures.
- `looptap-4`: cut `html`, `analyze`, Modal deploy.
- `looptap-5`: module path `github.com/TanGentleman/looptap` so `go install` works; README
  for newcomers with a runnable example on the bundled fixtures.

## Apply a branch upstream

```bash
# in kranti-sandbox: everything after the import commit
base=$(git log -1 --format=%H --grep='^Import looptap upstream' looptap-1)
git format-patch --relative=looptap "$base"..looptap-1 -o /tmp/looptap-1
# in a clean upstream clone at d8ab84b
git clone https://github.com/TanGentleman/looptap && cd looptap
git am /tmp/looptap-1/*.patch
go vet ./... && go test -race -count=1 ./...
```

For a stacked branch, use the previous branch as the base: `looptap-1..looptap-2`.
If `git am` rejects a hunk over line endings, retry with `git am --keep-cr`.

## Needs Tan

- Go floor raised to 1.26 (needed by current grpc via genai). If you want to keep 1.24
  users building from source, pin genai back instead.
- Cut a release from main (`cut-release.yml`) so `install.sh` users get `patterns`.
- Is tracers still alive and public? If not, the contract docs and fixtures can go.
- Close or rebase PR #8 and the Cursor drafts (#55-#58, #60-#62).
- Release workflows (`release.yml`, `release-smoke.yml`, `cut-release.yml`) were linted,
  not run.
