"""Opencode server, in a Modal Sandbox, pointed at a public GitHub repo.

Clones the target repo into a persistent `modal.Volume`, mounts it at
`/root/code`, then boots `opencode serve` inside a Modal Sandbox with an
encrypted tunnel on port 4096. Access the running server via the Web UI, the
TUI (`opencode attach`), or a raw `modal shell`.

The volume sticks around between runs — subsequent launches re-use the clone
and `git fetch` for updates instead of cloning from scratch.

Usage:

    # public repo, defaults to modal-labs/modal-examples@main
    python deploy/opencode_sandbox.py

    # pick a repo + ref + timeout
    python deploy/opencode_sandbox.py \\
        --repo TanGentleman/looptap --ref main --timeout 2h

Prereqs:

    * `modal token new` (or MODAL_TOKEN_ID / MODAL_TOKEN_SECRET in env).
    * A Modal secret named `opencode-secret` (override with `--secret`) holding:
        OPENCODE_SERVER_PASSWORD=<server auth password>
        # ...plus whichever provider key your opencode config needs, e.g.
        # ANTHROPIC_API_KEY=...  or  GOOGLE_API_KEY=...
      Create it at https://modal.com/secrets (type: Custom).
"""
from __future__ import annotations

import argparse
import re
import sys

import modal

APP_NAME_DEFAULT = "looptap-opencode"
OPENCODE_PORT = 4096
DEFAULT_REPO = "modal-labs/modal-examples"
DEFAULT_REF = "main"
DEFAULT_SECRET = "opencode-secret"
WORKDIR = "/root/code"

MINUTES = 60
HOURS = 60 * MINUTES


def define_image() -> modal.Image:
    """Debian slim + git + opencode on PATH. Nothing fancy."""
    return (
        modal.Image.debian_slim()
        .apt_install("curl", "git", "ca-certificates")
        .run_commands("curl -fsSL https://opencode.ai/install | bash")
        .env({"PATH": "/root/.opencode/bin:${PATH}"})
    )


def repo_slug(repo: str) -> str:
    """'modal-labs/modal-examples' -> 'modal-labs-modal-examples'."""
    slug = re.sub(r"[^a-z0-9-]+", "-", repo.lower()).strip("-")
    if not slug:
        raise ValueError(f"refusing to build a volume name from repo={repo!r}")
    return slug


def volume_name_for(repo: str) -> str:
    return f"looptap-opencode-{repo_slug(repo)}"


def prime_volume(image: modal.Image, volume: modal.Volume, repo: str, ref: str) -> None:
    """Clone the repo into the volume on first run; `git fetch` on reruns.

    Runs in a throwaway Sandbox so the writes are scoped + `volume.commit()`
    is a clean single-step after exit.
    """
    # bash scoping: if /root/code/.git exists, fast-forward to origin/<ref>;
    # otherwise clone fresh. Either way, the volume ends with a usable tree.
    script = f"""
set -eu
mkdir -p "{WORKDIR}"
cd "{WORKDIR}"
if [ -d .git ]; then
    echo "📦  existing clone found — fetching {ref}"
    git fetch --depth 1 origin "{ref}"
    git checkout -q "{ref}" 2>/dev/null || git checkout -q -B "{ref}" "origin/{ref}"
    git reset --hard "origin/{ref}"
else
    echo "📦  cloning {repo}@{ref}"
    # clone into a tmp dir then move contents in — works even if the mount
    # created an empty dir ahead of us.
    git clone --depth 1 --branch "{ref}" \\
        "https://github.com/{repo}.git" /tmp/code
    shopt -s dotglob
    mv /tmp/code/* "{WORKDIR}/"
    rmdir /tmp/code
fi
git -C "{WORKDIR}" --no-pager log -1 --oneline
"""
    print(f"🏖️  priming volume with {repo}@{ref}")
    with modal.enable_output():
        sb = modal.Sandbox.create(
            "bash",
            "-lc",
            script,
            image=image,
            volumes={WORKDIR: volume},
            timeout=10 * MINUTES,
        )
        sb.wait()
        code = sb.returncode
    # Persist the writes so the long-running sandbox boots a warm tree.
    volume.commit()
    if code != 0:
        raise RuntimeError(
            f"volume prime failed (exit {code}). Check the sandbox logs above."
        )


def create_sandbox(
    image: modal.Image,
    volume: modal.Volume,
    app: modal.App,
    secrets: list[modal.Secret],
    timeout: int,
) -> modal.Sandbox:
    """Boot `opencode serve` with the repo volume mounted at /root/code."""
    print("🏖️  creating opencode sandbox")
    with modal.enable_output():
        return modal.Sandbox.create(
            "opencode",
            "serve",
            "--hostname=0.0.0.0",
            f"--port={OPENCODE_PORT}",
            "--log-level=DEBUG",
            "--print-logs",
            encrypted_ports=[OPENCODE_PORT],
            secrets=secrets,
            volumes={WORKDIR: volume},
            timeout=timeout,
            image=image,
            app=app,
            workdir=WORKDIR,
        )


def print_access_info(sandbox: modal.Sandbox, password_secret_name: str) -> None:
    tunnel = sandbox.tunnels()[OPENCODE_PORT]
    print()
    print("🏖️  access the sandbox directly:")
    print(f"\tmodal shell {sandbox.object_id}")
    print("🏖️  access the Web UI:")
    print(f"\t{tunnel.url}")
    print("\tUsername: opencode")
    print("🏖️  access the TUI:")
    print(
        f"\tOPENCODE_SERVER_PASSWORD=YOUR_PASSWORD opencode attach {tunnel.url}"
    )
    print("🏖️  recover the password:")
    print(
        "\tmodal shell --secret "
        f"{password_secret_name} "
        "--cmd 'env | grep OPENCODE_SERVER_PASSWORD='"
    )


def parse_timeout(raw: str) -> int:
    """'2h' / '90m' / bare int (hours). Returns seconds."""
    s = raw.strip().lower()
    if s.endswith("h"):
        minutes = int(s[:-1]) * 60
    elif s.endswith("m"):
        minutes = int(s[:-1])
    else:
        minutes = int(s) * 60
    if minutes < 1:
        raise argparse.ArgumentTypeError("timeout must be at least 1 minute")
    if minutes > 24 * 60:
        raise argparse.ArgumentTypeError("timeout cannot exceed 24 hours")
    return minutes * MINUTES


def main(
    repo: str,
    ref: str,
    timeout: int,
    app_name: str,
    password_secret_name: str,
    volume_name: str | None,
) -> None:
    app = modal.App.lookup(app_name, create_if_missing=True)
    image = define_image()

    vol_name = volume_name or volume_name_for(repo)
    print(f"🏖️  volume: {vol_name}")
    volume = modal.Volume.from_name(vol_name, create_if_missing=True)

    prime_volume(image, volume, repo, ref)

    password_secret = modal.Secret.from_name(password_secret_name)
    sandbox = create_sandbox(
        image=image,
        volume=volume,
        app=app,
        secrets=[password_secret],
        timeout=timeout,
    )
    print_access_info(sandbox, password_secret_name)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Run opencode in a Modal Sandbox, backed by a Volume + public git repo.",
    )
    parser.add_argument(
        "--repo",
        default=DEFAULT_REPO,
        help=f"GitHub repo in owner/name form (must be public). Default: {DEFAULT_REPO}",
    )
    parser.add_argument(
        "--ref",
        default=DEFAULT_REF,
        help=f"Git ref (branch/tag/SHA) to check out. Default: {DEFAULT_REF}",
    )
    parser.add_argument(
        "--timeout",
        default="2h",
        type=parse_timeout,
        help="Sandbox lifetime — e.g. '2h', '90m', or bare int (hours). Default: 2h",
    )
    parser.add_argument(
        "--app-name",
        default=APP_NAME_DEFAULT,
        help=f"Modal app name. Default: {APP_NAME_DEFAULT}",
    )
    parser.add_argument(
        "--secret",
        dest="password_secret_name",
        default=DEFAULT_SECRET,
        help=(
            f"Modal secret holding OPENCODE_SERVER_PASSWORD (+ provider keys). "
            f"Default: {DEFAULT_SECRET}"
        ),
    )
    parser.add_argument(
        "--volume",
        dest="volume_name",
        default=None,
        help="Override the Modal Volume name. Default: derived from --repo.",
    )

    args = parser.parse_args()

    try:
        main(
            repo=args.repo,
            ref=args.ref,
            timeout=args.timeout,
            app_name=args.app_name,
            password_secret_name=args.password_secret_name,
            volume_name=args.volume_name,
        )
    except RuntimeError as e:
        print(f"error: {e}", file=sys.stderr)
        sys.exit(1)
