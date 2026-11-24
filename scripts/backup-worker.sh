#!/bin/sh

set -eu

CONFIG_DIR="${PROXFIN_CONF:-/home/proxfin/conf}"
SCRIPT_DIR="/opt/proxfin-backup"
SYNC_SCRIPT="${SCRIPT_DIR}/github_config_sync.py"
BACKUP_INTERVAL="${BACKUP_INTERVAL_SECONDS:-300}"
READY_MARKER="${CONFIG_DIR}/.github-backup-ready"

case "$BACKUP_INTERVAL" in
    ''|*[!0-9]*)
        echo "[Backup Worker] Invalid backup interval"
        exit 1
        ;;
esac

if [ "$BACKUP_INTERVAL" -lt 60 ]; then
    echo "[Backup Worker] Minimum interval is 60 seconds"
    exit 1
fi

echo "[Backup Worker] Started"
echo "[Backup Worker] Interval: ${BACKUP_INTERVAL} seconds"

trap 'exit 0' TERM INT

while true; do

    if python3 "$SYNC_SCRIPT" validate \
        --config-dir "$CONFIG_DIR"; then

        if [ -f "$READY_MARKER" ]; then

            if python3 "$SYNC_SCRIPT" backup \
                --config-dir "$CONFIG_DIR"; then

                echo "[Backup Worker] Backup check completed"

            else

                echo "[Backup Worker] Backup failed; will retry"

            fi

        else

            if python3 "$SYNC_SCRIPT" authorization-status; then

                if python3 "$SYNC_SCRIPT" backup \
                    --config-dir "$CONFIG_DIR" \
                    --initialize; then

                    echo "[Backup Worker] Initialization completed"

                else

                    echo "[Backup Worker] Initialization failed"

                fi

            else

                echo "[Backup Worker] Waiting for initialization authorization"

            fi

        fi

    else

        echo "[Backup Worker] Configuration not ready"

    fi

    sleep "$BACKUP_INTERVAL" &
    wait "$!" || true

done
