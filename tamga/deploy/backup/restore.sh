#!/usr/bin/env bash
# Tamga PostgreSQL Restore Script
# Usage: ./restore.sh <dump-file> [--db tamga] [--host localhost] [--port 5432] [--user tamga]
#
# Restores a pg_dump custom-format file created by backup.sh.
# WARNING: This DROPS and RECREATES the target database.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ $# -lt 1 ]; then
    echo "Usage: $0 <dump-file.sql.gz> [--db tamga] [--host localhost] [--port 5432] [--user tamga]"
    echo ""
    echo "Available backups:"
    ls -1t "$SCRIPT_DIR/dumps"/tamga-*.sql.gz 2>/dev/null | head -10 || echo "  (none found in dumps/)"
    exit 1
fi

DUMP_FILE="$1"
shift

if [ ! -f "$DUMP_FILE" ]; then
    echo "ERROR: Dump file not found: $DUMP_FILE"
    exit 1
fi

DB="${TAMGA_DB:-tamga}"
HOST="${TAMGA_DB_HOST:-localhost}"
PORT="${TAMGA_DB_PORT:-5432}"
USER="${TAMGA_DB_USER:-tamga}"

# Parse optional flags
while [ $# -gt 0 ]; do
    case "$1" in
        --db)   DB="$2"; shift 2 ;;
        --host) HOST="$2"; shift 2 ;;
        --port) PORT="$2"; shift 2 ;;
        --user) USER="$2"; shift 2 ;;
        *)      echo "Unknown flag: $1"; exit 1 ;;
    esac
done

# ── Confirmation ─────────────────────────────────────────────────────────────

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                    TAMGA DATABASE RESTORE                    ║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo "║  Database : $(printf '%-48s' "$DB")║"
echo "║  Host     : $(printf '%-48s' "$HOST")║"
echo "║  Port     : $(printf '%-48s' "$PORT")║"
echo "║  Dump     : $(printf '%-48s' "$(basename "$DUMP_FILE")")║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo "║  WARNING: This will DROP the database and recreate it.      ║"
echo "║  All current data will be permanently lost.                 ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""
read -r -p "Type 'yes' to confirm restore: " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo "Restore cancelled."
    exit 0
fi

# ── Pre-flight ──────────────────────────────────────────────────────────────

if ! command -v pg_restore &>/dev/null; then
    echo "ERROR: pg_restore not found in PATH. Install PostgreSQL client tools."
    exit 1
fi

# ── Restore ─────────────────────────────────────────────────────────────────

echo ""
echo "[$(date '+%H:%M:%S')] Dropping and recreating database '$DB'..."

PGPASSWORD="${TAMGA_DB_PASSWORD:-}" dropdb \
    --host="$HOST" \
    --port="$PORT" \
    --username="$USER" \
    --if-exists \
    "$DB"

PGPASSWORD="${TAMGA_DB_PASSWORD:-}" createdb \
    --host="$HOST" \
    --port="$PORT" \
    --username="$USER" \
    --owner="$USER" \
    "$DB"

echo "[$(date '+%H:%M:%S')] Restoring from $(basename "$DUMP_FILE")..."

PGPASSWORD="${TAMGA_DB_PASSWORD:-}" pg_restore \
    --host="$HOST" \
    --port="$PORT" \
    --username="$USER" \
    --dbname="$DB" \
    --clean \
    --if-exists \
    --no-owner \
    --no-acl \
    --single-transaction \
    --verbose \
    "$DUMP_FILE" 2>&1

RESTORE_EXIT=$?

if [ $RESTORE_EXIT -ne 0 ]; then
    echo ""
    echo "WARNING: pg_restore exited with code $RESTORE_EXIT (some errors may be non-fatal, e.g. 'does not exist' for DROP IF EXISTS)."
else
    echo ""
    echo "[$(date '+%H:%M:%S')] Restore complete — database '$DB' restored successfully."
fi
