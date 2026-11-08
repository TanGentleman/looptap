# looptap roadmap

looptap reads Claude Code and Codex transcripts from disk, flags rough patches
(correction loops, failing tools, repeated tool calls) with seven offline detectors, and
writes sessions, turns and signals to SQLite. `patterns` then turns failures that repeat
across sessions into CLAUDE.md/AGENTS.md rules. For people who use coding agents daily and
want to see where the agent keeps tripping.

## Status (upstream main d8ab84b, checked 2026-11-08)

- Upstream main builds on Go 1.24.5; this stack needs Go 1.26+ and is checked on
  Go 1.27.3 (go.dev tarball, sha256 checked): `go vet`, `go test -race`,
  `scripts/smoke-db.sh`, actionlint v1.7.12, shellcheck, shfmt v3.14.1.
- Before the stack: the Codex parser was a stub, `parse <dir>` ignored files outside
  `.claude/projects/`, `CLAUDE_CONFIG_DIR` and `isMeta` lines were not handled, and
  the seven detectors had no tests (10% statement coverage in `internal/signal`).
- `scripts/install.sh` installs v0.7.0 (2026-04-30), which has no `patterns`,
  `seed-contract-fixture` or Codex parser.
- `[signals]` and `[phrases]` in `config.toml` are parsed but no code reads them.
- Dead links: `https://github.com/TanGentleman/tracers` (404 without auth; the
  contract docs depend on it), schema `$id`s on `looptap.dev` (no DNS record; only
  used as keys in `internal/rule/contract_test.go`).
- Open upstream PRs: #59 dependabot (applied on `-1`), #8 (Gemini JSON parsing),
  drafts #55-#58, #60-#62.

## Signal vs noise

Keep: `parse` -> `signal` (`run` does both) -> `query` / `patterns`, plus `info`. The
parser and detector interfaces, SQLite schema, `metadata.json` for datasette.

Cut on `-4`: `html` (asks an agent to write a branch report; never reads transcripts),
`.github/workflows/html-report.yml`, the opencode configs, `deploy/` (Modal app),
`scripts/setup.sh`, `scripts/deploy.sh`, `example.env`. About 2,700 lines.

Cut on `-6` (optional; stop at `-5` to keep them): `advise` and `analyze`, the two
commands that need `GOOGLE_API_KEY`. `analyze` sends a CLAUDE.md file to Gemini and never
reads transcripts; `advise` sends signal counts to Gemini for CLAUDE.md rules, which
`patterns` already writes offline. Gone with them: `internal/advise`, `internal/analyze`,
the `[advise]` config section (old configs still load) and genai with 25 indirect deps
(`go.mod` keeps 4 direct, 3 indirect). The `tracers.analyze/v1` schemas and fixtures stay:
they describe the removed hosted `/v1/analyze`, not `looptap analyze`'s output.

Left alone, candidates for a later cut:
- `seed-contract-fixture`, `docs/schemas`, `testdata/contracts`: only matter to tracers.

## Stack

Each branch is based on the one before it. All pass `go vet`, `go test -race` and
`scripts/smoke-db.sh` after `git am` on a fresh upstream clone.

- `looptap-1`: import, Go 1.26 + current deps, actions bump (#59), this file.
- `looptap-2`: `parse <dir>` sniffs files outside `.claude/projects/`; default source
  honors `CLAUDE_CONFIG_DIR`; skip `isMeta` and `isCompactSummary` user lines.
- `looptap-3`: Codex rollout parser (`sessions/`, `archived_sessions/`, `CODEX_HOME`),
  shell-output header stripped so "Exit code: 0" isn't a failure; fixture + tests.
- `looptap-4`: cut `html` and the Modal deploy.
- `looptap-5`: module path `github.com/TanGentleman/looptap` (for `go install`), version
  from build info, README example on the bundled fixtures, this file updated.
- `looptap-6` (optional): cut `advise`, `analyze`, their config and genai. Root command
  built in `newRootCmd` so a test pins which commands exist.
- `looptap-7`: jsonschema v6.0.3; CI and release on Go 1.27.3, alpine 3.24,
  upload-artifact v7.0.2, download-artifact v8.0.2, actionlint v1.7.12, shfmt
  v3.14.1; table tests for the seven detectors and an end-to-end test of the README
  example; README rewritten; ARCHITECTURE matches the detectors; CI runs the quick
  start. Needs `-6`.
- `looptap-8`: README cut to 27 words; details moved to IMPLEMENTATION.md.

## Apply a branch upstream

```bash
# in kranti-sandbox: everything after the import commit
base=$(git log -1 --format=%H --grep='^Import looptap upstream' origin/looptap-1)
git format-patch --relative=looptap "$base"..origin/looptap-1 -o /tmp/looptap-1
# in a clean upstream clone at d8ab84b
git clone https://github.com/TanGentleman/looptap && cd looptap
git am /tmp/looptap-1/*.patch
go vet ./... && go test -race -count=1 ./...
```

For a stacked branch, use the previous branch as the base: `origin/looptap-1..origin/looptap-2`.
For the whole stack, `"$base"..origin/looptap-8` (or `..origin/looptap-5` to keep
`advise` and `analyze`; `-7` and `-8` sit on `-6`).
If `git am` rejects a hunk over line endings, retry with `git am --keep-cr`.

## Needs Tan

- Cut a release from main (`cut-release.yml`) once the stack lands, so `install.sh`
  and `go install ...@latest` users get `patterns` and the Codex parser. The
  release workflows were linted, not run.
- Run `looptap parse ~/.codex/sessions --db /tmp/c.db` on a real machine once; the
  Codex fixture is hand-written from the Rust types. `.jsonl.zst` rollouts are
  skipped (reading them needs a zstd dependency).
- `[signals]` and `[phrases]` config keys do nothing. Wire them into the detectors or
  delete them from `internal/config` and ARCHITECTURE.md.
- Go floor is 1.26: `golang.org/x/text` v0.43 (via jsonschema, test-only) declares
  go 1.26.0.
- `-6` removes `advise` and `analyze`; skip `-6` and `-7` if you still use them.
- Is tracers still alive and public? If not, `docs/contracts.md`, `docs/schemas`,
  `testdata/contracts` and `seed-contract-fixture` can go.
- Close or rebase PR #8 (moot after `-6`) and the drafts (#55-#58, #60-#62); `-4`
  deletes `deploy/`, which #57 edits.
