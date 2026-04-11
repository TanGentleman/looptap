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
looptap html       # hand a branch to claude, get a shareable HTML report back
```

`advise` and `analyze` need a Gemini API key — set `GOOGLE_API_KEY`. `html` shells out to the `claude` CLI in headless mode, so you'll need that on your PATH (override with `LOOPTAP_CLAUDE_BIN`).

```bash
looptap html --repo /path/to/repo --branch current --output report.html --force
```

Repo and branch also read from `LOOPTAP_REPO_PATH` and `LOOPTAP_BRANCH` (`current` | `default` | a branch name). Without `--force` you get a confirmation prompt showing the resolved repo and branch before anything runs.

Browse the DB with datasette:

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

---

Deeper dive: [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, config, all the knobs.
