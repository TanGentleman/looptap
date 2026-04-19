"""Modal deployment for looptap.

Three hats:
- wraps `looptap analyze` behind a FastAPI endpoint (the original stub)
- lists git clones pre-indexed into the `looptap-repos` Modal volume
- fans an HTML branch report out to a disposable Modal sandbox running opencode

Secrets: GOOGLE_API_KEY lives in the `looptap-secrets` Modal secret (populated
by scripts/setup.sh). We forward it as both GOOGLE_API_KEY (for looptap's Gemini
wrapper) and GOOGLE_GENERATIVE_AI_API_KEY (what opencode's google provider
reads).

Repo convention inside the `looptap-repos` volume: one git clone per path
`<owner>/<name>`. Drop one in with `modal volume put looptap-repos <local-clone>
<owner>/<name>` (or bootstrap from a warm-up function) and it lights up on
GET /repos.
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

BINARY_SRC = Path(__file__).parent / "bin" / "looptap"
SAMPLE_SRC = Path(__file__).parent / "claude.sample.md"
OPENCODE_CFG_SRC = Path(__file__).parent / "opencode.hosted.json"

BINARY_DST = "/opt/looptap/looptap"
SAMPLE_DST = "/root/.claude/CLAUDE.md"
OPENCODE_CFG_DST = "/opt/looptap/opencode.hosted.json"

# owner/name — no slashes, no traversal, friendly with git remote slugs.
_REPO_SLUG_RE = re.compile(r"[A-Za-z0-9._-]+/[A-Za-z0-9._-]+")

app = modal.App(APP_NAME)

repos_volume = modal.Volume.from_name(REPOS_VOLUME_NAME, create_if_missing=True)

image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install("ca-certificates", "git")
    .pip_install("fastapi==0.115.0")
    .add_local_file(str(BINARY_SRC), BINARY_DST, copy=True)
    .add_local_file(str(SAMPLE_SRC), SAMPLE_DST, copy=True)
    .dockerfile_commands(f"RUN chmod +x {BINARY_DST}")
)

# Sandbox image: opencode (installed from the upstream one-liner), git for
# refreshing the clone, plus the looptap binary so the sandbox can reuse the
# html command's opencode wiring instead of re-implementing the prompt.
sandbox_image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install("ca-certificates", "curl", "git", "unzip")
    .run_commands(
        "curl -fsSL https://opencode.ai/install | bash",
        "ln -sf /root/.opencode/bin/opencode /usr/local/bin/opencode",
    )
    .add_local_file(str(BINARY_SRC), BINARY_DST, copy=True)
    .add_local_file(str(OPENCODE_CFG_SRC), OPENCODE_CFG_DST, copy=True)
    .dockerfile_commands(f"RUN chmod +x {BINARY_DST}")
)


def _subprocess_env() -> dict[str, str]:
    """Mirror GOOGLE_API_KEY into GOOGLE_GENERATIVE_AI_API_KEY so both
    looptap (Gemini SDK) and opencode (google provider) are happy from the
    same secret.
    """
    key = os.environ.get("GOOGLE_API_KEY") or os.environ.get("GOOGLE_GENERATIVE_AI_API_KEY", "")
    return {
        **os.environ,
        "GOOGLE_API_KEY": key,
        "GOOGLE_GENERATIVE_AI_API_KEY": key,
        "HOME": "/root",
    }


def _run_analyze(file_path: str, as_json: bool) -> tuple[int, str, str]:
    """Shell out to looptap analyze. Returns (exit_code, stdout, stderr)."""
    cmd = [BINARY_DST, "analyze", "--file", file_path]
    if as_json:
        cmd.append("--json")
    proc = subprocess.run(cmd, env=_subprocess_env(), capture_output=True, text=True, timeout=120)
    return proc.returncode, proc.stdout, proc.stderr


def _validate_repo_slug(repo: str) -> str:
    """Accept only `owner/name` with safe chars. Returns the cleaned slug."""
    trimmed = repo.strip().strip("/")
    if not _REPO_SLUG_RE.fullmatch(trimmed):
        raise ValueError(f"repo must match 'owner/name' with [A-Za-z0-9._-] (got {repo!r})")
    return trimmed


def _list_indexed_repos() -> list[dict[str, Any]]:
    """Walk the mounted volume and collect <owner>/<name> entries. We reload
    the volume first so a `modal volume put` from outside this container
    becomes visible without redeploying.
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
            is_git = (repo_dir / ".git").is_dir() or (repo_dir / "HEAD").is_file()
            out.append({
                "repo": f"{owner_dir.name}/{repo_dir.name}",
                "path": str(repo_dir),
                "is_git": is_git,
            })
    return out


def _build_sandbox_script(repo: str, branch: str) -> str:
    """Refresh the cached clone, hardlink-clone it into /tmp (so concurrent
    callers don't stomp a shared working tree), pin the branch, and hand off
    to `looptap html`. The per-sandbox working copy is disposable — the
    container dies with the request.
    """
    src = shlex.quote(f"{REPOS_MOUNT}/{repo}")
    br = shlex.quote(branch)
    opencode_cfg = shlex.quote(OPENCODE_CFG_DST)
    binary = shlex.quote(BINARY_DST)
    return (
        "set -euo pipefail\n"
        f"git -C {src} fetch --all --prune --tags\n"
        "work=$(mktemp -d /tmp/looptap-work.XXXXXX)\n"
        f"git clone --local {src} \"$work/repo\"\n"
        "cd \"$work/repo\"\n"
        "git fetch origin --prune\n"
        f"git checkout {br} 2>/dev/null || git checkout -b {br} origin/{br}\n"
        f"{binary} html --agent opencode --is-sandbox "
        f"--opencode-config {opencode_cfg} "
        f"--repo . --branch {br} --force\n"
    )


def _run_opencode_sandbox(repo: str, branch: str) -> tuple[int, str, str]:
    """Spawn a sandbox, run the refresh+checkout+looptap-html script, return
    (exit_code, stdout, stderr). Stdout is the opencode-generated HTML.
    """
    key = os.environ.get("GOOGLE_API_KEY", "")
    sandbox = modal.Sandbox.create(
        "bash", "-c", _build_sandbox_script(repo, branch),
        image=sandbox_image,
        volumes={REPOS_MOUNT: repos_volume},
        secrets=[
            modal.Secret.from_name(SECRET_NAME),
            # opencode's google provider reads GOOGLE_GENERATIVE_AI_API_KEY;
            # the looptap-secrets bundle only has GOOGLE_API_KEY, so mirror it.
            modal.Secret.from_dict({"GOOGLE_GENERATIVE_AI_API_KEY": key}),
        ],
        app=app,
        timeout=SANDBOX_TIMEOUT,
    )
    try:
        sandbox.wait()
        stdout = sandbox.stdout.read()
        stderr = sandbox.stderr.read()
        return sandbox.returncode, stdout, stderr
    finally:
        sandbox.terminate()


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
            "  GET  /repos                          list repos indexed into the looptap-repos volume\n"
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

    @api.post("/analyze-repo")
    def analyze_repo(payload: dict[str, Any] = Body(...)):
        raw_repo = payload.get("repo") or ""
        branch = (payload.get("branch") or "main").strip()
        try:
            repo = _validate_repo_slug(raw_repo)
        except ValueError as exc:
            raise HTTPException(status_code=400, detail=str(exc))
        if not branch:
            raise HTTPException(status_code=400, detail="branch must not be empty")

        repos_volume.reload()
        if not (Path(REPOS_MOUNT) / repo).is_dir():
            raise HTTPException(
                status_code=404,
                detail=(
                    f"{repo!r} not found in volume {REPOS_VOLUME_NAME!r}. "
                    f"Index it with `modal volume put {REPOS_VOLUME_NAME} "
                    f"<local-clone> {repo}`."
                ),
            )
        if not os.environ.get("GOOGLE_API_KEY"):
            raise HTTPException(
                status_code=500,
                detail=f"GOOGLE_API_KEY missing from '{SECRET_NAME}'.",
            )

        code, out, err = _run_opencode_sandbox(repo, branch)
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
