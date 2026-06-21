#!/usr/bin/env bash
# Tamga PostgreSQL Backup Script
# Usage: ./backup.sh [--db tamga] [--host localhost] [--port 5432] [--user tamga]
#
# Creates a compressed, timestamped pg_dump in ./dumps/.
# Designed for cron scheduling: 0 2 * * * /path/to/backup.sh
#
# Retention: keeps last 7 daily dumps by default. Set BACKUP_RETENTION_DAYS to override.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DUMP_DIR="${SCRIPT_DIR}/dumps"
LOG_FILE="${SCRIPT_DIR}/backup.log"

DB="${TAMGA_DB:-tamga}"
HOST="${TAMGA_DB_HOST:-localhost}"
PORT="${TAMGA_DB_PORT:-5432}"
USER="${TAMGA_DB_USER:-tamga}"
RETENTION="${BACKUP_RETENTION_DAYS:-7}"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

# ── Pre-flight ──────────────────────────────────────────────────────────────

mkdir -p "$DUMP_DIR"

if ! command -v pg_dump &>/dev/null; then
    log "ERROR: pg_dump not found in PATH. Install PostgreSQL client tools."
    exit 1
fi

# ── Dump ────────────────────────────────────────────────────────────────────

TIMESTAMP="$(date '+%Y%m%d-%H%M%S')"
DUMP_FILE="${DUMP_DIR}/tamga-${TIMESTAMP}.sql.gz"

log "Starting backup: ${DUMP_FILE}"

PGPASSWORD="${TAMGA_DB_PASSWORD:-}" pg_dump \
    --host="$HOST" \
    --port="$PORT" \
    --username="$USER" \
    --dbname="$DB" \
    --format=custom \
    --compress=9 \
    --no-owner \
    --no-acl \
    --file="$DUMP_FILE" 2>&1 | tee -a "$LOG_FILE"

if [ ${PIPESTATUS[0]} -ne 0 ]; then
    log "ERROR: pg_dump failed (exit code ${PIPESTATUS[0]})"
    rm -f "$DUMP_FILE"
    exit 2
fi

DUMP_SIZE="$(du -h "$DUMP_FILE" | cut -f1)"
log "Backup complete: ${DUMP_FILE} (${DUMP_SIZE})"

# ── Retention ───────────────────────────────────────────────────────────────

DELETED=0
for f in $(ls -1t "$DUMP_DIR"/tamga-*.sql.gz 2>/dev/null | tail -n +$((RETENTION + 1))); do
    log "Removing old backup: $(basename "$f")"
    rm -f "$f"
    DELETED=$((DELETED + 1))
done

if [ "$DELETED" -gt 0 ]; then
    log "Retention cleanup: removed ${DELETED} old backup(s)"
fi

log "Backup job finished. $(ls "$DUMP_DIR"/tamga-*.sql.gz 2>/dev/null | wc -l) backup(s) retained."
