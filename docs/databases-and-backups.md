# Databases and backups

Managed databases are persistent, volume-backed containers alongside your apps.
They handle credential generation, backups, public access for external tools, and
the same metrics/logs visibility as your applications. Package:
`internal/store/database.go`, `internal/reconcile/database/controller.go`,
`internal/api/backups.go`.

## Supported engines

Eight database engines are available, each with a sensible default version that
can be overridden per database:

| Engine | Default Version | Use case |
| --- | --- | --- |
| Postgres | 16 | Relational database, ACID, fully featured SQL |
| MySQL | 8 | Relational database, InnoDB storage engine |
| MariaDB | 11 | MySQL-compatible fork, lightweight option |
| MongoDB | 7 | Document database, flexible schema |
| Redis | 7 | In-memory cache, sessions, task queues |
| ClickHouse | 24.8 | Time-series and analytics database |
| Dragonfly | v1.27.1 | Redis-compatible cache, lower memory footprint |
| KeyDB | latest | Redis-compatible cache, cluster-aware |

Every managed database is automatically assigned:

- **A persistent volume** for durable storage, surviving container restarts
- **Generated credentials** at creation time: unique username and password
  (Postgres, MySQL, MariaDB, MongoDB, ClickHouse all require auth; Redis,
  Dragonfly, and KeyDB generate a password but can run without it since they
  are typically internal to your network)
- **Internal network isolation** by default: only accessible from within the
  Docker network, same as your apps. No external port binding unless you
  explicitly enable it

## Creating a database

### Via the dashboard

1. Navigate to **Databases** and click **Create database**
2. Choose an engine and version
3. Optionally set memory and CPU resource limits (defaults: no limit)
4. Optionally enable public access to connect external tools (see below)
5. Optionally configure a backup schedule (see Backups below)
6. Click **Create**

### Via the CLI

Create with all required fields:

```bash
levelrail-cli databases create \
  --name my-postgres \
  --engine postgres \
  --version 16
```

Or use the interactive wizard:

```bash
levelrail-cli databases create --interactive
```

The interactive mode guides you through engine selection, resource limits, and
backup scheduling in one flow.

List all databases:

```bash
levelrail-cli databases list
```

View one database's status:

```bash
levelrail-cli databases get my-postgres
```

The status output includes current node assignment, credentials visibility (if
set), attached apps, and any reconciliation conditions (e.g., "waiting for
volume", "container running").

## Connecting to a database

### From within your apps

Apps in the same Levelrail deployment reach any database via an internal DNS
name and the standard container port. The platform injects connection strings
as environment variables automatically when you attach an app to a database
(dashboard: app > Database > Attach, or `levelrail-cli apps set-database`).

Example connection string for Postgres named `my-postgres`:

```
postgres://username:password@my-postgres:5432/dbname
```

(where `dbname` defaults to the database name; you control it in the reconciler
configuration for each engine).

### From outside your network (public access)

Enable public access to expose a database port to the host machine, letting you
connect external tools like pgAdmin, TablePlus, or RedisInsight.

**Via the dashboard:**

1. Open a database detail page
2. Go to **Public access**
3. Toggle **Enable public access**
4. Either request a specific port (1024-65535, excluding 80, 443, 8080, 9443) or
   let the system auto-assign from the range 20000-20999
5. Click **Enable**

The response includes the public port assigned. Note: public access bypasses
network isolation; only enable for databases that need external access.

Once enabled, you can connect from your machine:

```bash
psql -h 127.0.0.1 -p 25432 -U username -d my-postgres
```

(Port 25432 is an example; use the actual port shown in the dashboard).

## Resource limits

Set memory and CPU limits to bound a database's resource consumption.

**Via the dashboard:**

1. Open a database detail page
2. Go to **Resources**
3. Set memory (e.g., 256Mi, 1Gi) and CPU (e.g., 0.5, 1.0)
4. Click **Apply**

Get a recommendation based on historical usage:

```bash
levelrail-cli databases resource-recommendation my-postgres
```

This analyzes past peak usage and suggests a safe limit for CPU and memory.

## Backups

Backups are copies of a database dumped to S3-compatible storage. They can be
triggered manually, scheduled on a cron expression, verified without restore,
restored into the original database (destructive), or restored into a new
database (non-destructive clone).

### Setting up a backup target

A backup target is an S3-compatible destination bucket where dumps are stored.
Before creating a backup schedule, set up at least one target.

**Via the CLI:**

```bash
# Create a new backup target
levelrail-cli backup-targets create \
  --name my-s3-backup \
  --provider aws \
  --bucket my-bucket \
  --access-key-id YOUR_KEY \
  --secret-access-key YOUR_SECRET

# Or for a Cloudflare R2 (S3-compatible) bucket
levelrail-cli backup-targets create \
  --name my-r2-backup \
  --provider r2 \
  --bucket my-bucket \
  --endpoint https://xxx.r2.cloudflarestorage.com \
  --access-key-id YOUR_KEY \
  --secret-access-key YOUR_SECRET
```

Valid providers: `aws` (resolves region from bucket), `r2` (requires endpoint),
`custom` (any S3-compatible API, requires endpoint).

List targets:

```bash
levelrail-cli backup-targets list
```

Test a target's connectivity without uploading:

```bash
levelrail-cli backup-targets test <target-id>
```

### Manual backups

Start a backup right now:

```bash
levelrail-cli backups trigger my-postgres --target my-s3-backup
```

The command returns immediately; the actual dump and upload happen in the
background. Check progress:

```bash
levelrail-cli backups list my-postgres
```

Output shows each backup attempt with status (running, success, failed), start
time, finish time, size in bytes, and checksum.

### Scheduled backups

Configure a recurring backup via cron expression. This example backs up every
day at 3 AM:

```bash
levelrail-cli backups schedule set my-postgres \
  --target my-s3-backup \
  --cron "0 3 * * *"
```

Cron format: minute hour day-of-month month day-of-week (standard 5-field
expression).

Add retention policies:

```bash
levelrail-cli backups schedule set my-postgres \
  --target my-s3-backup \
  --cron "0 3 * * *" \
  --retain 7 \
  --retain-days 30
```

- `--retain 7`: keep the 7 most recent successful backups; delete older ones
- `--retain-days 30`: delete backups older than 30 days, independent of count

Both are optional and independent. If neither is set, all backups are kept.

Remove a schedule:

```bash
levelrail-cli backups schedule clear my-postgres
```

### Verifying backups

Verify a backup without restoring it. This re-downloads the backup, checks its
checksum and size, and runs a lightweight structural validation:

```bash
levelrail-cli backups verify my-postgres --backup <backup-id>
```

Check verification history:

```bash
levelrail-cli backups verifications my-postgres --backup <backup-id>
```

### Restoring a database

**Destructive restore** (overwrites live data):

```bash
levelrail-cli backups restore my-postgres --backup <backup-id>
```

You must confirm by typing the database name. To skip the interactive prompt:

```bash
levelrail-cli backups restore my-postgres \
  --backup <backup-id> \
  --confirm my-postgres
```

During restore, the database container is stopped, data is replaced, and the
container restarts. Apps referencing this database will see a brief connection
loss.

**Non-destructive restore** (clone into a new database):

```bash
levelrail-cli backups restore-as-new my-postgres \
  --backup <backup-id> \
  --new-name my-postgres-staging
```

Optionally specify a different engine version for the new database:

```bash
levelrail-cli backups restore-as-new my-postgres \
  --backup <backup-id> \
  --new-name my-postgres-staging \
  --version 16
```

This is the safe way to test a migration against real data or spin up a staging
replica without risking the production database.

## Observability

Every database exposes metrics and logs, the same as apps.

### Logs

View database logs (container stdout/stderr):

```bash
# Search with text filter
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/databases/my-postgres/logs?q=error"

# Stream live logs (use Server-Sent Events)
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/databases/my-postgres/logs/stream"
```

Logs are indexed for full-text search. The dashboard includes a log viewer on
each database's Logs tab.

### Metrics

Query CPU, memory, disk I/O, and network I/O:

```bash
# Last 1 hour, 15-second resolution
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/databases/my-postgres/metrics?since=1h"
```

Available metrics: `cpu`, `memory`, `disk_read`, `disk_write`, `network_in`,
`network_out`. The dashboard includes a metrics dashboard on each database's
Metrics tab.

## Not built yet

- **Postgres extensions (pgvector, PostGIS, TimescaleDB):** These are available
  in the container images but are not yet surfaced as declarative options in
  the database spec or the UI. You can enable them manually inside the running
  container for now.
- **MySQL/MariaDB replication setup:** Multi-instance replication is not yet a
  managed feature.
- **Database migrations and schema management tools:** The platform manages
  containers and volumes; you still run schema migrations yourself from your app
  or a separate CLI tool.
- **Automated backups for app volumes:** App services can have volumes backed up
  the same way, but it's a separate command group (`app-volume-backups`) rather
  than integrated into the same backup flow.
