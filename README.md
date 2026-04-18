# looptap

`strace` but for vibes. Reads your coding agent transcripts, finds the rough patches — correction loops, tool call death spirals, dead ends — and dumps it to SQLite.

## Try it

```bash
curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/install.sh | bash
```

```bash
looptap run        # parse transcripts + detect signals
looptap info       # what'd we find?
looptap advise     # ask Gemini for CLAUDE.md fixes based on your signals
looptap analyze    # quality-review your ~/.claude/CLAUDE.md
looptap html       # hand a branch to a coding agent, get a shareable HTML report back
```

`advise` and `analyze` need a Gemini API key — set `GOOGLE_API_KEY`. `html` shells out to a coding-agent CLI in headless mode: either `claude` (the default, override with `LOOPTAP_CLAUDE_BIN`) or `opencode` (override with `LOOPTAP_OPENCODE_BIN`). Pick with `--agent`.

```bash
# default: Claude Code
looptap html --repo /path/to/repo --branch current --output report.html --force

# opencode with the built-in read-only config (ANTHROPIC_API_KEY from env)
looptap html --agent opencode --repo /path/to/repo --output report.html --force

# opencode with your own config (model, provider creds, tool allowlist, etc.)
looptap html --agent opencode --opencode-config ./opencode.json \
  --repo /path/to/repo --output report.html --force
```

Without `--opencode-config`, looptap ships an embedded default that allows `read`/`glob`/`grep`/`list`/`bash` and denies `edit`/`webfetch`/`websearch` — enough for the agent to poke at git without wandering off. Copy [`internal/htmlreport/opencode.default.json`](internal/htmlreport/opencode.default.json) as a starting point for your own.

Repo, branch, agent, and opencode config also read from `LOOPTAP_REPO_PATH`, `LOOPTAP_BRANCH` (`current` | `default` | a branch name), `LOOPTAP_AGENT` (`claude` | `opencode`), and `LOOPTAP_OPENCODE_CONFIG`. Without `--force` you get a confirmation prompt showing the resolved repo, branch, and agent before anything runs.

Browse the DB with datasette:

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

## Branch reports from CI

[`.github/workflows/html-report.yml`](.github/workflows/html-report.yml) drives `looptap html` via opencode on `google/gemini-3.1-flash-lite-preview`. Dispatch it from the Actions tab with a branch name; the rendered HTML falls out as a workflow artifact. The agent config is inlined in the workflow (written from the trusted default branch, not from the branch under inspection) — read/glob/grep/list plus a narrow git-read-only bash allowlist, with edit/webfetch/websearch denied.

**Setup**: add a `GOOGLE_GENERATIVE_AI_API_KEY` repository secret before the first run — **Settings → Secrets and variables → Actions → New repository secret**, name `GOOGLE_GENERATIVE_AI_API_KEY`, value your Google AI Studio key. That's the variable opencode's google provider reads; the workflow refuses to run without it. Swap the provider/model in the workflow's inline config if you'd rather use a different key (remember to rename the secret too).

---

Deeper dive: [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, config, all the knobs.
