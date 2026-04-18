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

# opencode — allowed tools, model, and provider keys live in the JSON config
looptap html --agent opencode --opencode-config ./opencode.json \
  --repo /path/to/repo --output report.html --force
```

Repo, branch, agent, and opencode config also read from `LOOPTAP_REPO_PATH`, `LOOPTAP_BRANCH` (`current` | `default` | a branch name), `LOOPTAP_AGENT` (`claude` | `opencode`), and `LOOPTAP_OPENCODE_CONFIG`. Without `--force` you get a confirmation prompt showing the resolved repo, branch, and agent before anything runs.

Browse the DB with datasette:

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

---

Deeper dive: [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, config, all the knobs.
