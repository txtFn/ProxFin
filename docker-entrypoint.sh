#!/bin/sh

set -eu

CONFIG_DIR="${THREADFIN_CONF:-${PROXFIN_CONF:-/home/proxfin/conf}}"
SCRIPT_DIR="/opt/proxfin-backup"
SYNC_SCRIPT="${SCRIPT_DIR}/github_config_sync.py"
READY_MARKER="${CONFIG_DIR}/.github-backup-ready"

echo "[Startup] Initializing ProxFin"

# Preserve compatibility with legacy Threadfin volumes.
if [ -z "${THREADFIN_CONF:-}" ] &&
   [ -n "$(ls -A /home/threadfin/conf 2>/dev/null)" ] &&
   [ -z "$(ls -A "$CONFIG_DIR" 2>/dev/null)" ]; then

    CONFIG_DIR="/home/threadfin/conf"
    echo "[Startup] Using legacy Threadfin configuration"

fi

export PROXFIN_CONF="$CONFIG_DIR"
READY_MARKER="${CONFIG_DIR}/.github-backup-ready"

mkdir -p "$CONFIG_DIR"
rm -f "$READY_MARKER"

# Verify required environment variables.
: "${GITHUB_BACKUP_TOKEN:?Missing GitHub backup token}"
: "${GITHUB_BACKUP_REPO:?Missing GitHub backup repository}"
: "${BACKUP_ENCRYPTION_KEY:?Missing encryption key}"

echo "[Startup] Checking remote configuration backup"

RESTORE_STATUS=0

python3 "$SYNC_SCRIPT" restore \
    --config-dir "$CONFIG_DIR" || RESTORE_STATUS=$?

case "$RESTORE_STATUS" in

    0)
        touch "$READY_MARKER"
        echo "[Startup] Configuration restored successfully"
        ;;

    10)
        echo "[Startup] No existing remote backup"
        echo "[Startup] Waiting for first-backup authorization"
        ;;

    *)
        echo "[Startup] Restoration failed"
        echo "[Startup] Refusing to start with potentially incorrect configuration"
        exit 1
        ;;

esac

# Start the periodic backup worker.
echo "[Startup] Starting backup worker"

sh "${SCRIPT_DIR}/backup-worker.sh" &

# Start ProxFin as PID 1.
echo "[Startup] Starting ProxFin"

exec "${PROXFIN_BIN}/proxfin" \
    -port="${THREADFIN_PORT:-$PROXFIN_PORT}" \
    -bind="${THREADFIN_BIND_IP_ADDRESS:-$PROXFIN_BIND_IP_ADDRESS}" \
    -config="$CONFIG_DIR" \
    -debug="${THREADFIN_DEBUG:-$PROXFIN_DEBUG}" \
    "$@"
