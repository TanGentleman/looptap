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
looptap html       # generate an HTML branch report for the dev team
```

`advise` and `analyze` need a Gemini API key — set `GOOGLE_API_KEY`.
`html` shells out to `claude -p` in headless mode, so you'll need the
[Claude Code CLI](https://code.claude.com/docs/en/cli-reference) on PATH.

```bash
# explicit: point at a repo and branch, skip the prompt
looptap html --repo ~/code/myproj --branch default --output report.html --force

# or set it and forget it
export LOOPTAP_REPO_PATH=~/code/myproj
export LOOPTAP_BRANCH=feature/new-thing
looptap html -o report.html
```

Browse the DB with datasette:

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

---

Deeper dive: [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, config, all the knobs.
