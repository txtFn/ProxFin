
#!/bin/sh
# Periodically synchronize ProxFin configuration to GitHub.

set -u

conf="${1:-${PROXFIN_CONF:-/home/proxfin/conf}}"
interval="${GITHUB_BACKUP_INTERVAL:-300}"

case "$interval" in
    ''|*[!0-9]*|0)
        echo "Invalid backup interval; using 300 seconds."
        interval=300
        ;;
esac

echo "GitHub backup worker started."

while :; do
    sleep "$interval"

    if [ ! -f "${conf}/.github-backup-ready" ]; then
        echo "Checking first-backup authorization..."

        if python3 /usr/local/bin/github_config_sync.py \
            authorization-status --config-dir "$conf"; then

            echo "First-backup authorization detected."

            if python3 /usr/local/bin/github_config_sync.py \
                backup --initialize --config-dir "$conf"; then
                echo "First GitHub backup initialized."
            else
                echo "First backup failed; will retry."
            fi
        else
            echo "First-backup authorization unavailable."
        fi

        continue
    fi

    if ! python3 /usr/local/bin/github_config_sync.py \
        backup --config-dir "$conf"; then
        echo "GitHub configuration backup skipped or failed."
    fi
done
