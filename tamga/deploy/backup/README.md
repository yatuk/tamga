# Tamga Backup & Restore

PostgreSQL backup and disaster recovery for Tamga proxy.

## Quick Start

```bash
# 1. Set database password
export TAMGA_DB_PASSWORD="your-db-password"

# 2. Run backup
./backup.sh

# 3. Verify
ls -lh dumps/
```

## Backup (`backup.sh`)

Creates a compressed, timestamped `pg_dump` in `dumps/`.

**Usage:**
```bash
./backup.sh [--db tamga] [--host localhost] [--port 5432] [--user tamga]
```

**Environment variables:**

| Variable | Default | Description |
|----------|---------|-------------|
| `TAMGA_DB` | `tamga` | Database name |
| `TAMGA_DB_HOST` | `localhost` | PostgreSQL host |
| `TAMGA_DB_PORT` | `5432` | PostgreSQL port |
| `TAMGA_DB_USER` | `tamga` | Database user |
| `TAMGA_DB_PASSWORD` | (empty) | Database password |
| `BACKUP_RETENTION_DAYS` | `7` | Days to keep |

**Automation (cron):**
```cron
# Daily at 2:00 AM
0 2 * * * cd /opt/tamga/deploy/backup && TAMGA_DB_PASSWORD=xyz ./backup.sh
```

## Restore (`restore.sh`)

Restores a database from a `pg_dump` custom-format file.

**Usage:**
```bash
./restore.sh dumps/tamga-20260617-020000.sql.gz [--db tamga] [--host localhost]
```

**WARNING:** This DROPS and RECREATES the target database. All current data will be permanently lost. The script requires typing `yes` to confirm.

## Dump Format

- **Format:** `pg_dump` custom format (`.sql.gz`) — compressed, supports parallel restore
- **Contents:** Schema + data, no ownership/ACL statements (safe to restore to different user)
- **Naming:** `tamga-YYYYMMDD-HHMMSS.sql.gz`
- **Retention:** 7 daily backups by default (configurable via `BACKUP_RETENTION_DAYS`)

## Disaster Recovery Procedure

1. **Verify the backup is intact:**
   ```bash
   gunzip -c dumps/tamga-YYYYMMDD-HHMMSS.sql.gz | head -20
   # Should show valid SQL with CREATE/COPY statements
   ```

2. **Provision a new PostgreSQL instance** (or use the existing one if only data was lost).

3. **Restore:**
   ```bash
   export TAMGA_DB_PASSWORD="your-db-password"
   ./restore.sh dumps/tamga-YYYYMMDD-HHMMSS.sql.gz
   ```

4. **Verify the restore:**
   ```bash
   PGPASSWORD="$TAMGA_DB_PASSWORD" psql -h localhost -U tamga -d tamga -c "SELECT count(*) FROM request_log;"
   ```

5. **Restart the proxy** so it reconnects to the restored database:
   ```bash
   docker compose restart tamga-proxy
   # or
   systemctl restart tamga-proxy
   ```

## Validation Checklist

Run these after any backup/restore change:

- [ ] `backup.sh` produces a non-empty `.sql.gz` file in `dumps/`
- [ ] `restore.sh` successfully restores to a fresh database
- [ ] Row counts match between source and restored DB for key tables (`request_log`, `security_events`, `audit_log`)
- [ ] Proxy starts and serves requests against the restored database
- [ ] Cron or scheduled backup is configured and tested
