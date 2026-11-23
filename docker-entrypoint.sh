#!/bin/sh
# ProxFin Docker entrypoint with GitHub configuration restoration.
set -e

conf="${PROXFIN_CONF}"

# Preserve compatibility with existing Threadfin volumes.
if [ -n "$(ls -A /home/threadfin/conf 2>/dev/null)" ] && \
   [ -z "$(ls -A "${conf}" 2>/dev/null)" ]; then
    conf=/home/threadfin/conf
    echo "Using existing Threadfin settings in ${conf}"
fi

conf="${THREADFIN_CONF:-$conf}"

# Restore only when GitHub integration is configured.
if [ -n "${GITHUB_BACKUP_TOKEN:-}" ] && \
   [ -n "${GITHUB_BACKUP_REPO:-}" ]; then

    # Never overwrite an existing local configuration at startup.
    if [ -z "$(ls -A "${conf}" 2>/dev/null)" ]; then
        echo "Checking GitHub for ProxFin configuration backup..."

    # Capture the actual restoration exit code.
    restore_status=0

    python3 /usr/local/bin/github_config_sync.py \
        restore --config-dir "${conf}" || restore_status=$?

    case "$restore_status" in
        0)
            # Successful restoration: enable periodic backups.
            touch "${conf}/.github-backup-ready"
            echo "GitHub configuration restored; backups enabled."
            ;;
        10)
            # Fresh installation: no remote backup yet.
            echo "No GitHub backup exists yet."
            echo "Automatic backups remain disabled until initialization."
            ;;
        *)
            echo "ERROR: GitHub restoration failed (exit $restore_status)."
            exit 1
            ;;
    esac

    else
        echo "Existing local configuration found; skipping GitHub restore."
    fi
fi

# Start the worker whenever GitHub integration is configured.
# The worker itself waits until backup initialization is complete.
if [ -n "${GITHUB_BACKUP_TOKEN:-}" ] && \
   [ -n "${GITHUB_BACKUP_REPO:-}" ]; then

    echo "Starting GitHub configuration backup worker..."

    /usr/local/bin/github_backup_worker.sh "${conf}" &
fi

# Preserve ProxFin's original PID 1 behavior.
exec "${PROXFIN_BIN}/proxfin" \
    -port="${THREADFIN_PORT:-$PROXFIN_PORT}" \
    -bind="${THREADFIN_BIND_IP_ADDRESS:-$PROXFIN_BIND_IP_ADDRESS}" \
    -config="${conf}" \
    -debug="${THREADFIN_DEBUG:-$PROXFIN_DEBUG}" \
    "$@"
