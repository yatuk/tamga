# Backup and restore

`backup.sh` and `restore.sh` dump and restore the proxy's PostgreSQL database
with `pg_dump` and `pg_restore`.

## What is and is not backed up

Backed up: one database, schema and data. That includes `request_logs` with
all of its monthly partitions, `daily_stats`, `audit_log`, the policy history
(`policy_revisions`, `policy_proposals`), `incident_lifecycle`, `saved_hunts`,
API keys and the other tables created by `deploy/migrations/`.

Not backed up by these scripts:

- the policy file the proxy reads (`TAMGA_POLICY_PATH`),
- Redis (rate-limit, budget and cache state),
- NATS streams,
- environment variables and secrets.

## Requirements

- `pg_dump`, `pg_restore`, `dropdb` and `createdb` on the `PATH`, from a
  PostgreSQL client at least as new as the server (the Compose stack runs
  PostgreSQL 16).
- Network access to the database. In `deploy/docker-compose.yml` PostgreSQL is
  on the internal network only and is not published to the host, so
  `localhost:5432` does not reach it. Publish the port, or run the scripts
  from a container on that network.
- For `restore.sh`: a role that may drop and create the database.

## Settings

Both scripts read the same environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `TAMGA_DB` | `tamga` | Database name |
| `TAMGA_DB_HOST` | `localhost` | PostgreSQL host |
| `TAMGA_DB_PORT` | `5432` | PostgreSQL port |
| `TAMGA_DB_USER` | `tamga` | Database user |
| `TAMGA_DB_PASSWORD` | (empty) | Database password, passed as `PGPASSWORD` |
| `BACKUP_RETENTION_DAYS` | `7` | `backup.sh` only: how many dumps to keep |

These are the scripts' own variables. The proxy does not read them; it takes
its connection string from `TAMGA_DB_URL`.

`restore.sh` also accepts `--db`, `--host`, `--port` and `--user` after the
dump file. `backup.sh` takes no arguments (the usage line in its header
mentions flags it does not parse); use the environment variables.

## Backup

```bash
export TAMGA_DB_PASSWORD="..."
./backup.sh
ls -lh dumps/
```

The script writes `dumps/tamga-YYYYMMDD-HHMMSS.sql.gz` next to itself and
appends to `backup.log` in the same directory. On a `pg_dump` failure it
removes the partial file and exits with status 2.

After a successful dump it deletes all but the newest
`BACKUP_RETENTION_DAYS` dump files. The number counts files, not days: with
one backup a day the default keeps a week, with two a day it keeps three and
a half days.

### The dump format

The file is in `pg_dump` custom format (`--format=custom --compress=9`,
`--no-owner --no-acl`). Despite the `.sql.gz` name it is neither plain SQL nor
a gzip file: `gunzip` and `psql` cannot read it. Read it with `pg_restore`:

```bash
pg_restore --list dumps/tamga-YYYYMMDD-HHMMSS.sql.gz | head -20
```

### Scheduling

```cron
# Daily at 02:00. Keep the password out of the crontab: read it from a file
# only the backup user can read.
0 2 * * * cd /opt/tamga/deploy/backup && TAMGA_DB_PASSWORD="$(cat /etc/tamga/db-password)" ./backup.sh
```

The paths are examples.

## Restore

```bash
export TAMGA_DB_PASSWORD="..."
./restore.sh dumps/tamga-YYYYMMDD-HHMMSS.sql.gz [--db tamga] [--host localhost] [--port 5432] [--user tamga]
```

Run without arguments, it lists the ten newest dumps.

**This drops the target database and creates it again. Everything in it is
lost.** The script prints the target and asks you to type `yes` first.

It then runs `dropdb --if-exists`, `createdb --owner=<user>`, and
`pg_restore --clean --if-exists --no-owner --no-acl --single-transaction`.
The restore is one transaction and the script runs with `set -e`: at the
first error it stops and the database is left empty.

`dropdb` fails while anything is connected to the database, so stop the proxy
first.

## Recovery procedure

1. Check that the dump is readable:

   ```bash
   pg_restore --list dumps/tamga-YYYYMMDD-HHMMSS.sql.gz | head -20
   ```

2. Stop the proxy. With the Compose stack, from the repository root:

   ```bash
   docker compose --env-file .env -f deploy/docker-compose.yml stop proxy
   ```

3. Restore:

   ```bash
   export TAMGA_DB_PASSWORD="..."
   ./restore.sh dumps/tamga-YYYYMMDD-HHMMSS.sql.gz
   ```

4. If the dump was taken by an older version, apply the migrations that were
   added since, in order, from `deploy/`. Nothing records which migrations a
   database has had, so you need to know the version the dump came from.
   Do not loop over all of them: `001`, `010` and `011` fail when run a
   second time. `017` adds the `policy_name`, `policy_version` and
   `output_risk` columns and an index to `request_logs`; `018` removes the
   row-level security that `011` added.

   ```bash
   psql "$TAMGA_DB_URL" -v ON_ERROR_STOP=1 -f migrations/017_request_record.up.sql
   psql "$TAMGA_DB_URL" -v ON_ERROR_STOP=1 -f migrations/018_drop_tenant_rls.up.sql
   ```

5. Check the row counts:

   ```bash
   PGPASSWORD="$TAMGA_DB_PASSWORD" psql -h localhost -U tamga -d tamga \
     -c "SELECT count(*) FROM request_logs;" \
     -c "SELECT count(*) FROM audit_log;"
   ```

6. Start the proxy:

   ```bash
   docker compose --env-file .env -f deploy/docker-compose.yml start proxy
   ```

## Checklist after changing either script

- [ ] `backup.sh` writes a dump that `pg_restore --list` can read
- [ ] `restore.sh` restores it into an empty database
- [ ] Row counts of `request_logs`, `daily_stats` and `audit_log` match the source
- [ ] `\d+ request_logs` lists the same partitions as the source
- [ ] The proxy starts against the restored database and `GET /api/v1/health/detailed`, called with the admin key, reports `database: "connected"`
