"""Modal deployment for looptap.

Wraps the `looptap analyze` subcommand behind a FastAPI endpoint. A sample
~/.claude/CLAUDE.md ships inside the image so callers can hit the route with
zero payload and still get a real answer back.

Secrets: GOOGLE_API_KEY lives in the `looptap-secrets` Modal secret (populated
by scripts/setup.sh). Modal injects it into the function container's env
(https://modal.com/docs/guide/secrets), and we forward it explicitly into the
subprocess environment when shelling out to the looptap binary — os.environ
inside the function already has it, but being explicit keeps the subprocess
contract obvious.
"""
from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import modal

APP_NAME = "looptap"
FUNCTION_NAME = "web"
SECRET_NAME = "looptap-secrets"

BINARY_SRC = Path(__file__).parent / "bin" / "looptap"
SAMPLE_SRC = Path(__file__).parent / "claude.sample.md"

BINARY_DST = "/opt/looptap/looptap"
SAMPLE_DST = "/root/.claude/CLAUDE.md"

app = modal.App(APP_NAME)

image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install("ca-certificates")
    .pip_install("fastapi==0.115.0")
    .add_local_file(str(BINARY_SRC), BINARY_DST, copy=True)
    .add_local_file(str(SAMPLE_SRC), SAMPLE_DST, copy=True)
    .dockerfile_commands(f"RUN chmod +x {BINARY_DST}")
)


def _run_analyze(file_path: str, as_json: bool) -> tuple[int, str, str]:
    """Shell out to looptap analyze. Returns (exit_code, stdout, stderr).

    GOOGLE_API_KEY is forwarded explicitly so the subprocess contract is
    readable; Modal's Secret.from_name attachment already populates it in
    os.environ inside the function container.
    """
    api_key = os.environ.get("GOOGLE_API_KEY", "")
    env = {
        **os.environ,
        "GOOGLE_API_KEY": api_key,
        "HOME": "/root",
    }
    cmd = [BINARY_DST, "analyze", "--file", file_path]
    if as_json:
        cmd.append("--json")
    proc = subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=120)
    return proc.returncode, proc.stdout, proc.stderr


@app.function(
    image=image,
    secrets=[modal.Secret.from_name(SECRET_NAME)],
    timeout=180,
)
@modal.asgi_app()
def web():
    from fastapi import FastAPI, HTTPException, Query
    from fastapi.responses import JSONResponse, PlainTextResponse

    api = FastAPI(title="looptap")

    @api.get("/", response_class=PlainTextResponse)
    def root() -> str:
        return (
            "looptap — hosted on Modal.\n"
            "  GET /healthz            liveness\n"
            "  GET /analyze            run `looptap analyze` on the sample CLAUDE.md\n"
            "  GET /analyze?json=1     raw JSON findings\n"
        )

    @api.get("/healthz")
    def healthz():
        binary_ok = shutil.which(BINARY_DST) is not None or Path(BINARY_DST).is_file()
        sample_ok = Path(SAMPLE_DST).is_file()
        key_set = bool(os.environ.get("GOOGLE_API_KEY"))
        return {
            "binary": binary_ok,
            "sample_claude_md": sample_ok,
            "google_api_key": key_set,
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
            import json as _json

            try:
                return JSONResponse(content=_json.loads(out))
            except _json.JSONDecodeError:
                return JSONResponse(
                    status_code=502,
                    content={"error": "looptap did not return valid JSON", "stdout": out},
                )
        return PlainTextResponse(out)

    return api
