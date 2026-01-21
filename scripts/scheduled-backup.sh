#!/usr/bin/env bash
#
# MAIA Scheduled Backup Script
# Designed for cron/systemd timer execution
#
# Usage: ./scheduled-backup.sh
#
# This script wraps backup.sh with additional features:
#   - Lock file to prevent concurrent runs
#   - Logging to file
#   - Health check after backup
#   - Notification support (Slack, webhook)
#   - Exit codes for monitoring
#
# Environment Variables:
#   MAIA_DATA_DIR         Data directory to backup (default: ./data)
#   MAIA_BACKUP_DIR       Backup destination (default: ./backups)
#   BACKUP_COMPRESS       Enable compression (default: true)
#   BACKUP_ENCRYPT        Enable encryption (default: false)
#   BACKUP_RETENTION_DAYS Days to keep backups (default: 30)
#   BACKUP_LOG_DIR        Log directory (default: ./logs)
#   SLACK_WEBHOOK_URL     Slack webhook for notifications
#   WEBHOOK_URL           Generic webhook for notifications
#   MAIA_URL              MAIA instance URL for health check
#
# Exit Codes:
#   0 - Success
#   1 - General error
#   2 - Lock file exists (another backup running)
#   3 - Backup failed
#   4 - Health check failed
#
# Cron Example (daily at 2 AM):
#   0 2 * * * /path/to/scheduled-backup.sh >> /var/log/maia-backup.log 2>&1
#

set -euo pipefail

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Configuration with defaults
DATA_DIR="${MAIA_DATA_DIR:-./data}"
BACKUP_DIR="${MAIA_BACKUP_DIR:-./backups}"
COMPRESS="${BACKUP_COMPRESS:-true}"
ENCRYPT="${BACKUP_ENCRYPT:-false}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
LOG_DIR="${BACKUP_LOG_DIR:-./logs}"
MAIA_URL="${MAIA_URL:-http://localhost:8080}"

# Derived paths
LOCK_FILE="/tmp/maia-backup.lock"
LOG_FILE="${LOG_DIR}/backup_$(date +%Y%m%d).log"
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')

# Ensure log directory exists
mkdir -p "$LOG_DIR"

# Logging function
log() {
    local level="$1"
    shift
    echo "[${TIMESTAMP}] [${level}] $*" | tee -a "$LOG_FILE"
}

# Send notification
send_notification() {
    local status="$1"
    local message="$2"
    local color="${3:-#36a64f}"  # Default green

    # Slack notification
    if [[ -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        local payload
        payload=$(cat <<EOF
{
    "attachments": [{
        "color": "${color}",
        "title": "MAIA Backup ${status}",
        "text": "${message}",
        "ts": $(date +%s)
    }]
}
EOF
)
        curl -s -X POST -H 'Content-type: application/json' \
            --data "$payload" "$SLACK_WEBHOOK_URL" &> /dev/null || true
    fi

    # Generic webhook notification
    if [[ -n "${WEBHOOK_URL:-}" ]]; then
        local payload
        payload=$(cat <<EOF
{
    "event": "maia_backup",
    "status": "${status}",
    "message": "${message}",
    "timestamp": "${TIMESTAMP}"
}
EOF
)
        curl -s -X POST -H 'Content-type: application/json' \
            --data "$payload" "$WEBHOOK_URL" &> /dev/null || true
    fi
}

# Acquire lock
acquire_lock() {
    if [[ -f "$LOCK_FILE" ]]; then
        local pid
        pid=$(cat "$LOCK_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            log "ERROR" "Another backup is already running (PID: $pid)"
            exit 2
        else
            log "WARN" "Stale lock file found, removing"
            rm -f "$LOCK_FILE"
        fi
    fi

    echo $$ > "$LOCK_FILE"
    trap 'rm -f "$LOCK_FILE"' EXIT
}

# Health check
health_check() {
    log "INFO" "Performing health check..."

    if curl -sf "${MAIA_URL}/health" > /dev/null 2>&1; then
        log "INFO" "Health check passed"
        return 0
    else
        log "WARN" "Health check failed (MAIA may not be running)"
        return 1
    fi
}

# Run backup
run_backup() {
    log "INFO" "Starting scheduled backup..."
    log "INFO" "Data directory: $DATA_DIR"
    log "INFO" "Backup directory: $BACKUP_DIR"

    local backup_args=(
        "--data-dir" "$DATA_DIR"
        "--output-dir" "$BACKUP_DIR"
        "--retention" "$RETENTION_DAYS"
    )

    if [[ "$COMPRESS" == "true" ]]; then
        backup_args+=("--compress")
    fi

    if [[ "$ENCRYPT" == "true" ]]; then
        backup_args+=("--encrypt")
    fi

    # Run backup script
    if "$SCRIPT_DIR/backup.sh" "${backup_args[@]}" >> "$LOG_FILE" 2>&1; then
        log "INFO" "Backup completed successfully"
        return 0
    else
        log "ERROR" "Backup failed"
        return 1
    fi
}

# Get backup stats
get_backup_stats() {
    local latest_backup
    latest_backup=$(find "$BACKUP_DIR" -name "maia_backup_*.tar*" -type f -mmin -60 | head -1)

    if [[ -n "$latest_backup" ]]; then
        local size
        size=$(du -h "$latest_backup" | cut -f1)
        echo "Latest backup: $(basename "$latest_backup") (${size})"
    else
        echo "No recent backup found"
    fi

    local total_backups
    total_backups=$(find "$BACKUP_DIR" -name "maia_backup_*.tar*" -type f | wc -l)
    echo "Total backups: ${total_backups}"

    local total_size
    total_size=$(du -sh "$BACKUP_DIR" 2>/dev/null | cut -f1 || echo "0")
    echo "Total backup size: ${total_size}"
}

# Main function
main() {
    log "INFO" "========================================"
    log "INFO" "MAIA Scheduled Backup"
    log "INFO" "========================================"

    # Acquire lock to prevent concurrent runs
    acquire_lock

    # Optional pre-backup health check
    if [[ "${CHECK_HEALTH_BEFORE:-false}" == "true" ]]; then
        if ! health_check; then
            log "WARN" "Pre-backup health check failed, continuing anyway"
        fi
    fi

    # Run backup
    local backup_status=0
    if ! run_backup; then
        backup_status=3
    fi

    # Get stats
    local stats
    stats=$(get_backup_stats)
    log "INFO" "Backup Statistics:"
    echo "$stats" | while read -r line; do
        log "INFO" "  $line"
    done

    # Send notification
    if [[ $backup_status -eq 0 ]]; then
        send_notification "SUCCESS" "Backup completed successfully.\n${stats}" "#36a64f"
        log "INFO" "Backup job completed successfully"
    else
        send_notification "FAILED" "Backup failed. Check logs for details." "#ff0000"
        log "ERROR" "Backup job failed"
    fi

    # Optional post-backup health check
    if [[ "${CHECK_HEALTH_AFTER:-false}" == "true" ]]; then
        if ! health_check; then
            log "ERROR" "Post-backup health check failed"
            if [[ $backup_status -eq 0 ]]; then
                backup_status=4
            fi
        fi
    fi

    log "INFO" "========================================"
    exit $backup_status
}

main "$@"
