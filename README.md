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

### Hosted on Modal

Want `looptap analyze` reachable over HTTP? `cp example.env .env`, fill in the creds, then:

```bash
./scripts/setup.sh   # preflights Gemini, upserts the looptap-secrets Modal secret
./scripts/deploy.sh  # builds a linux binary, deploys deploy/app.py, smokes /healthz
```

The endpoint is gated by [Modal proxy auth tokens](https://modal.com/docs/guide/webhook-proxy-auth) — a raw `curl` gets a 401. Create a pair at <https://modal.com/settings/proxy-auth-tokens> and send them on **every** request:

```bash
curl -H "Modal-Key: $MODAL_PROXY_TOKEN_ID" \
     -H "Modal-Secret: $MODAL_PROXY_TOKEN_SECRET" \
     "$URL/analyze"
```

Any client code that calls this server — scripts, CI jobs, another service — needs the same two headers. `GOOGLE_API_KEY` rides along via `Secret.from_name("looptap-secrets")` and the function forwards it into the subprocess env explicitly.

Browse the DB with datasette:

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

---

Deeper dive: [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, config, all the knobs.
