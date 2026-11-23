
#!/usr/bin/env python3
"""GitHub-backed configuration snapshots for ProxFin."""

import argparse
import base64
import json
import os
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

FILES = (
    "authentication.json",
    "pms.json",
    "settings.json",
    "xepg.json",
    "urls.json",
)

BACKUP_FILE = "proxfin-config.json"
AUTHORIZATION_FILE = "initialize-backup.json"


def request_github(method, path, payload=None):
    token = os.environ["GITHUB_BACKUP_TOKEN"]
    repo = os.environ["GITHUB_BACKUP_REPO"]

    if len(repo.split("/")) != 2:
        raise ValueError("Invalid GitHub repository setting")

    url = f"https://api.github.com/repos/{repo}/{path}"

    headers = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
        "User-Agent": "ProxFin-Config-Sync",
    }

    body = None

    if payload is not None:
        body = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"

    req = urllib.request.Request(
        url, data=body, headers=headers, method=method
    )

    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response)
    except urllib.error.HTTPError as exc:
        if exc.code == 404 and method == "GET":
            return None
        raise RuntimeError(
            f"GitHub request failed: HTTP {exc.code}"
        ) from None


def encoded_branch():
    branch = os.environ.get("GITHUB_BACKUP_BRANCH", "main")
    return urllib.parse.quote(branch, safe="")


def initialization_authorized():
    """Check explicit authorization in the private GitHub repository."""
    result = request_github(
        "GET",
        f"contents/{AUTHORIZATION_FILE}?ref={encoded_branch()}",
    )

    if result is None:
        return False

    document = json.loads(
        base64.b64decode(result["content"]).decode("utf-8")
    )

    return (
        isinstance(document, dict)
        and document.get("initialize") is True
    )


def fetch_backup():
    # Confirm that the private repository is accessible.
    repository = request_github("GET", "")

    if repository is None:
        raise RuntimeError(
            "GitHub backup repository is inaccessible"
        )

    branch = os.environ.get("GITHUB_BACKUP_BRANCH", "main")

    branch_result = request_github(
        "GET",
        f"branches/{encoded_branch()}",
    )

    if branch_result is None:
        raise RuntimeError(
            f"GitHub backup branch '{branch}' is inaccessible"
        )

    result = request_github(
        "GET",
        f"contents/{BACKUP_FILE}?ref={encoded_branch()}",
    )

    if result is None:
        return None, None

    document = json.loads(
        base64.b64decode(result["content"]).decode("utf-8")
    )

    return document, result["sha"]


def validate_files(files):
    if not isinstance(files, dict):
        raise ValueError("Invalid configuration snapshot")

    if set(files) != set(FILES):
        raise ValueError("Missing or unexpected configuration files")

    for name in FILES:
        content = files[name]

        if not isinstance(content, str) or not content.strip():
            raise ValueError(f"Empty configuration file: {name}")

        parsed = json.loads(content)

        if not isinstance(parsed, dict):
            raise ValueError(f"Invalid JSON object: {name}")


def read_local(config_dir):
    files = {}

    for name in FILES:
        path = config_dir / name

        if not path.is_file():
            raise FileNotFoundError(
                f"Missing configuration: {name}"
            )

        files[name] = path.read_text(encoding="utf-8")

    validate_files(files)

    # Reject completely empty default configurations.
    if all(json.loads(content) == {} for content in files.values()):
        raise ValueError(
            "Configuration is not initialized; "
            "refusing to back up empty defaults"
        )

    return files


def validate(config_dir):
    read_local(config_dir)
    print("Configuration validation successful")


def backup(config_dir, initialize=False):
    files = read_local(config_dir)
    existing, sha = fetch_backup()

    if initialize:
        if existing is not None:
            raise RuntimeError(
                "A GitHub backup already exists. "
                "Initialization cannot overwrite it."
            )

        if not initialization_authorized():
            raise RuntimeError(
                "First backup is not authorized in GitHub."
            )

    elif existing is None:
        raise RuntimeError(
            "No remote backup exists. "
            "Explicit initialization is required."
        )

    if existing is not None:
        if existing.get("version") != 1:
            raise ValueError("Unsupported remote backup version")

        validate_files(existing.get("files"))

        if existing["files"] == files:
            print("Configuration unchanged")
            return

    document = {
        "version": 1,
        "files": files,
    }

    encoded = base64.b64encode(
        json.dumps(document, sort_keys=True).encode("utf-8")
    ).decode("ascii")

    branch = os.environ.get("GITHUB_BACKUP_BRANCH", "main")

    payload = {
        "message": "Update ProxFin configuration backup",
        "content": encoded,
        "branch": branch,
    }

    if sha:
        payload["sha"] = sha

    request_github(
        "PUT",
        f"contents/{BACKUP_FILE}",
        payload,
    )

    if initialize:
        marker = config_dir / ".github-backup-ready"
        marker.touch()
        print("First backup initialized; automatic backups enabled.")

    print("Configuration backup successful")


def restore(config_dir):
    document, _ = fetch_backup()

    if document is None:
        print("No remote backup found; first backup not initialized")
        sys.exit(10)

    if document.get("version") != 1:
        raise ValueError("Unsupported backup version")

    files = document.get("files")
    validate_files(files)

    config_dir.mkdir(parents=True, exist_ok=True)

    staged = {}

    try:
        for name, content in files.items():
            with tempfile.NamedTemporaryFile(
                mode="w",
                encoding="utf-8",
                dir=config_dir,
                prefix=".proxfin-restore-",
                delete=False,
            ) as temp:
                temp.write(content)
                temp.flush()
                os.fsync(temp.fileno())
                staged[name] = Path(temp.name)

        for name, temporary in staged.items():
            os.replace(temporary, config_dir / name)

    finally:
        for temporary in staged.values():
            temporary.unlink(missing_ok=True)

    print("Configuration restoration successful")


def main():
    parser = argparse.ArgumentParser()

    parser.add_argument(
        "action",
        choices=[
            "validate",
            "backup",
            "restore",
            "authorization-status",
        ],
    )

    parser.add_argument(
        "--config-dir",
        default=os.environ.get(
            "PROXFIN_CONF", "/home/proxfin/conf"
        ),
    )

    parser.add_argument(
        "--initialize",
        action="store_true",
        help="Allow creation of the first remote backup",
    )

    args = parser.parse_args()
    config_dir = Path(args.config_dir)

    if args.initialize and args.action != "backup":
        parser.error("--initialize is only valid with backup")

    if args.action == "authorization-status":
        if initialization_authorized():
            print("First-backup authorization is enabled.")
            return

        print("First-backup authorization is not enabled.")
        sys.exit(11)

    if args.action == "validate":
        validate(config_dir)
    elif args.action == "backup":
        backup(config_dir, args.initialize)
    else:
        restore(config_dir)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(
            f"Configuration synchronization failed: {exc}",
            file=sys.stderr,
        )
        sys.exit(1)
