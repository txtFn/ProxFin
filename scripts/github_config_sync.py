#!/usr/bin/env python3
"""Encrypted GitHub configuration persistence for ProxFin."""

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

from cryptography.hazmat.primitives.ciphers.aead import AESGCM


FILES = (
    "authentication.json",
    "pms.json",
    "settings.json",
    "xepg.json",
    "urls.json",
)

BACKUP_FILE = "proxfin-config.json.enc"
AUTHORIZATION_FILE = "initialize-backup.json"
LEGACY_BACKUP_FILE = "proxfin-config.json"

VERSION = 1
AAD = b"proxfin-config-backup-v1"


def encode_json(document):
    return json.dumps(
        document,
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=False,
    ).encode("utf-8")


def encryption_key():
    value = os.environ.get("BACKUP_ENCRYPTION_KEY", "")

    if not value:
        raise RuntimeError("Missing BACKUP_ENCRYPTION_KEY")

    try:
        key = base64.b64decode(value, validate=True)
    except Exception:
        raise ValueError("Invalid encryption key encoding") from None

    if len(key) != 32:
        raise ValueError("Encryption key must contain 32 bytes")

    return key


def encrypt_snapshot(document):
    key = encryption_key()
    nonce = os.urandom(12)

    ciphertext = AESGCM(key).encrypt(
        nonce,
        encode_json(document),
        AAD,
    )

    return encode_json({
        "version": VERSION,
        "algorithm": "AES-256-GCM",
        "nonce": base64.b64encode(nonce).decode("ascii"),
        "ciphertext": base64.b64encode(ciphertext).decode("ascii"),
    })


def decrypt_snapshot(data):
    envelope = json.loads(data)

    if envelope.get("version") != VERSION:
        raise ValueError("Unsupported encryption version")

    if envelope.get("algorithm") != "AES-256-GCM":
        raise ValueError("Unsupported encryption algorithm")

    nonce = base64.b64decode(
        envelope["nonce"], validate=True
    )

    ciphertext = base64.b64decode(
        envelope["ciphertext"], validate=True
    )

    if len(nonce) != 12:
        raise ValueError("Invalid encryption nonce")

    try:
        plaintext = AESGCM(encryption_key()).decrypt(
            nonce, ciphertext, AAD
        )
    except Exception:
        raise RuntimeError(
            "Backup authentication or decryption failed"
        ) from None

    document = json.loads(plaintext)

    if document.get("version") != VERSION:
        raise ValueError("Unsupported snapshot version")

    validate_files(document.get("files"))

    return document


def request_github(method, path="", payload=None):
    token = os.environ["GITHUB_BACKUP_TOKEN"]
    repo = os.environ["GITHUB_BACKUP_REPO"]

    if len(repo.split("/")) != 2:
        raise ValueError("Invalid backup repository")

    url = f"https://api.github.com/repos/{repo}"

    if path:
        url += "/" + path

    headers = {
        "Authorization": f"Bearer {token}",
        "Accept": "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
        "User-Agent": "ProxFin-Encrypted-Backup",
    }

    body = None

    if payload is not None:
        body = encode_json(payload)
        headers["Content-Type"] = "application/json"

    request = urllib.request.Request(
        url,
        data=body,
        headers=headers,
        method=method,
    )

    try:
        with urllib.request.urlopen(
            request, timeout=60
        ) as response:
            return json.load(response)

    except urllib.error.HTTPError as exc:
        if exc.code == 404 and method == "GET":
            return None

        raise RuntimeError(
            f"GitHub request failed: HTTP {exc.code}"
        ) from None


def branch_name():
    return os.environ.get("GITHUB_BACKUP_BRANCH", "main")


def encoded_branch():
    return urllib.parse.quote(branch_name(), safe="")


def verify_repository():
    repository = request_github("GET")

    if repository is None:
        raise RuntimeError("Backup repository inaccessible")

    if repository.get("private") is not True:
        raise RuntimeError("Backup repository must be private")

    branch = request_github(
        "GET", f"branches/{encoded_branch()}"
    )

    if branch is None:
        raise RuntimeError("Backup branch inaccessible")


def get_remote_file(filename):
    return request_github(
        "GET",
        f"contents/{filename}?ref={encoded_branch()}",
    )


def decode_remote_file(result):
    if result.get("encoding") != "base64":
        raise RuntimeError(
            "Unsupported GitHub file encoding or file too large"
        )

    return base64.b64decode(result["content"])


def initialization_authorized():
    verify_repository()

    result = get_remote_file(AUTHORIZATION_FILE)

    if result is None:
        return False

    document = json.loads(decode_remote_file(result))

    return (
        isinstance(document, dict)
        and document.get("initialize") is True
    )


def fetch_backup():
    verify_repository()

    result = get_remote_file(BACKUP_FILE)

    if result is None:
        legacy = get_remote_file(LEGACY_BACKUP_FILE)

        if legacy is not None:
            raise RuntimeError(
                "Legacy plaintext backup detected; "
                "manual migration required"
            )

        return None, None

    document = decrypt_snapshot(
        decode_remote_file(result)
    )

    return document, result["sha"]


def validate_files(files):
    if not isinstance(files, dict):
        raise ValueError("Invalid configuration snapshot")

    if set(files) != set(FILES):
        raise ValueError(
            "Missing or unexpected configuration files"
        )

    parsed = {}

    for name in FILES:
        content = files[name]

        if not isinstance(content, str):
            raise ValueError(f"Invalid file: {name}")

        if not content.strip():
            raise ValueError(f"Empty file: {name}")

        value = json.loads(content)

        if not isinstance(value, dict):
            raise ValueError(f"Invalid JSON object: {name}")

        parsed[name] = value

    settings = parsed["settings.json"]
    uuid = settings.get("uuid")

    if not isinstance(uuid, str) or not uuid.strip():
        raise ValueError("Missing ProxFin UUID")

    providers = settings.get("files")

    if not isinstance(providers, dict):
        raise ValueError("Missing provider configuration")

    m3u = providers.get("m3u")

    if not isinstance(m3u, dict) or not m3u:
        raise ValueError("No configured M3U providers")

    return parsed


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

    return files


def read_stable_snapshot(config_dir):
    first = read_local(config_dir)
    second = read_local(config_dir)

    if first != second:
        raise RuntimeError(
            "Configuration changed during snapshot; retry later"
        )

    return second


def validate(config_dir):
    read_stable_snapshot(config_dir)
    print("Configuration validation successful")


def backup(config_dir, initialize=False):
    encryption_key()

    files = read_stable_snapshot(config_dir)

    existing, sha = fetch_backup()

    if initialize:
        if existing is not None:
            raise RuntimeError(
                "Existing backup cannot be initialized again"
            )

        if not initialization_authorized():
            raise RuntimeError(
                "First backup is not authorized"
            )

    elif existing is None:
        raise RuntimeError(
            "No remote backup exists; initialization required"
        )

    if existing is not None:
        previous = validate_files(existing["files"])
        current = validate_files(files)

        if (
            previous["settings.json"]["uuid"]
            != current["settings.json"]["uuid"]
        ):
            raise RuntimeError(
                "ProxFin UUID changed; refusing to overwrite backup"
            )

        previous_mappings = previous["xepg.json"]
        current_mappings = current["xepg.json"]

        removed_mappings = (
            set(previous_mappings) - set(current_mappings)
        )

        if removed_mappings:
            raise RuntimeError(
                "Channel mapping loss detected: "
                f"{len(removed_mappings)} existing mappings missing; "
                "refusing to overwrite remote backup"
            )

        if existing["files"] == files:
            print("Configuration unchanged")
            return

    document = {
        "version": VERSION,
        "files": files,
    }

    encrypted = encrypt_snapshot(document)

    if decrypt_snapshot(encrypted) != document:
        raise RuntimeError("Encryption verification failed")

    # Keep snapshots within the reliable GitHub Contents API
    # base64 response size range.
    if len(encrypted) > 1000000:
        raise RuntimeError(
            "Encrypted snapshot exceeds supported size"
        )

    payload = {
        "message": "Update encrypted ProxFin backup",
        "content": base64.b64encode(
            encrypted
        ).decode("ascii"),
        "branch": branch_name(),
    }

    if sha:
        payload["sha"] = sha

    request_github(
        "PUT",
        f"contents/{BACKUP_FILE}",
        payload,
    )

    verified, _ = fetch_backup()

    if verified != document:
        raise RuntimeError(
            "Remote backup verification failed"
        )

    if initialize:
        marker = config_dir / ".github-backup-ready"
        marker.touch()
        print("First encrypted backup initialized")

    print("Encrypted configuration backup successful")
    print(f"Encrypted snapshot size: {len(encrypted)} bytes")


def restore(config_dir):
    encryption_key()

    document, _ = fetch_backup()

    if document is None:
        print("No encrypted backup found")
        sys.exit(10)

    files = document["files"]
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
                staged[name] = Path(temp.name)

                temp.write(content)
                temp.flush()
                os.fsync(temp.fileno())

        for name, temporary in staged.items():
            os.replace(
                temporary,
                config_dir / name,
            )

        restored = read_local(config_dir)

        if restored != files:
            raise RuntimeError(
                "Restored configuration verification failed"
            )

    finally:
        for temporary in staged.values():
            temporary.unlink(missing_ok=True)

    print("Encrypted configuration restoration successful")


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
            "PROXFIN_CONF",
            "/home/proxfin/conf",
        ),
    )

    parser.add_argument(
        "--initialize",
        action="store_true",
    )

    args = parser.parse_args()
    config_dir = Path(args.config_dir)

    if args.initialize and args.action != "backup":
        parser.error(
            "--initialize is only valid with backup"
        )

    if args.action == "authorization-status":
        if initialization_authorized():
            print("First-backup authorization enabled")
            return

        print("First-backup authorization unavailable")
        sys.exit(11)

    elif args.action == "validate":
        validate(config_dir)

    elif args.action == "backup":
        backup(config_dir, args.initialize)

    elif args.action == "restore":
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
