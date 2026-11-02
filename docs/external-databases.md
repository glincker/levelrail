---
title: "External databases"
description: Connect a database that already runs somewhere else, or adopt a running container, without moving or changing any data, and point apps at it.
"og:url": "https://docs.levelrail.com/external-databases"
---

An external database is one Levelrail connects to but does not run. Use it when
a database already holds real data and you want apps to use it where it is:
a Postgres container left over from another platform, a managed cloud
database, or a server on your LAN. Nothing is copied, restarted or modified.
You can move the data into a [managed database](/managing-databases) later, or never.

## What it does and does not do

- Stores the address, user, database name, TLS mode and Docker network. The
  password goes into the same envelope-encrypted secrets store as every other
  credential and is never returned by the API.
- Shows the database in the Databases list with an **External** badge. Actions
  that only make sense for a database Levelrail runs (stop, start, resources,
  version change, major upgrade, backups, public access) are not offered.
- Injects the same connection env into apps as a managed database does
  (`host`, `port`, `username`, `password`, `database`, `url`).
- Opens the [database viewer](/database-viewer) against it (Postgres, MySQL,
  MariaDB), read-only unless an admin uses the guarded write route.
- Removing it deletes only Levelrail's record and the stored password. The
  database is never contacted.

## Connect an existing database

Dashboard: Databases, **Connect existing**. Pick "A container on this server"
or "Enter the connection details", fill in the form, run **Test connection**,
then save.

CLI:

```
printf '%s\n' "$PGPASSWORD" | levelrail-cli databases connect \
  --name legacy --engine postgres --host 10.0.0.5 --user app \
  --database appdb --tls-mode require --password-stdin --json
levelrail-cli databases connect --engine postgres --host 10.0.0.5 --test --password-stdin
```

Defaults per engine: port 5432, 3306, 27017 or 6379; TLS "if available" for
Postgres, MySQL and MariaDB, off for MongoDB and Redis.

## Adopt a running container

```
levelrail-cli databases adopt --list
levelrail-cli databases adopt --container old-pg --name legacy \
  --user postgres --database appdb --password-stdin
```

Adopting lists running containers whose image is Postgres (including
pgvector, PostGIS, TimescaleDB), MySQL, MariaDB, MongoDB or Redis. Containers
Levelrail created itself are never offered. The host, port, Docker network and
default user are prefilled. The password is never read from the container: you
supply it.

Adopt never stops, restarts, recreates or modifies the container, and never
attaches it to another network. Connections are made by container name over the
container's own Docker network, because a helper on the default bridge cannot
reach a container on another user-defined network through the host's address.
The network is stored with the record and re-resolved on every check, so the
record follows the container when it is recreated.

## Health

A read-only probe runs from a short-lived helper container every five minutes
(`APP_EXTERNAL_DB_PROBE_INTERVAL`, `0` disables) and on demand
(`levelrail-cli databases probe NAME`, or **Check now**). The helper runs one
`SELECT` style identity query and is removed straight away.

| Status | Meaning | What to do |
| --- | --- | --- |
| reachable | The query ran. | Nothing. |
| slow | Answered, but slower than `APP_EXTERNAL_DB_SLOW_MS` (1500). | Check load and the network path. |
| auth_failed | User or password refused, or the host is not allowed. | Check them and the server's access rules (pg_hba.conf). |
| tls_error | TLS does not match what the server expects. | Switch TLS between off, if available and required. |
| unreachable | Nothing answered. | Check host, port, network and that it is running. |

## Using it from an app

```yaml
services:
  web:
    env:
      DATABASE_URL: { from: legacy.url }
```

or `levelrail-cli apps connect web legacy --field url`.
The app container is attached to the database's Docker network when it is
created and on every reconcile; the database container is not changed. Env
values resolve when the container is created, so restart an app that was
already running.

## Safety rules

- Hosts that are link-local or cloud metadata addresses (169.254.0.0/16,
  fe80::/10, 100.100.100.200, metadata names) are rejected, including when a
  hostname resolves to one. Set `APP_EXTERNAL_DB_ALLOW_LINK_LOCAL=true` to opt in.
  Loopback and `localhost` are always rejected: inside a helper they point at
  the helper itself.
- Creating, testing, adopting and revealing a password require an admin
  token. Revealing is recorded in the audit log. Listing needs only read.
- The control plane opens no firewall port for an external database.
- The only statements sent to it are the probe query and what an operator runs
  through the viewer. Writes go through the same admin-only route that
  requires the database name as confirmation.
- The MCP server exposes `list_external_databases` (read-only). No MCP tool can
  create, change or probe one.

## Hosted database cookbook

Levelrail takes the host, port, user, database and TLS mode as separate fields. It does not parse a connection string, so split the vendor's URL into those fields. It does build a URL for your app from them: Postgres gets `?sslmode=<mode>`, MongoDB gets `?tls=true` and Redis gets the `rediss://` scheme when TLS mode is `require`. A `mongodb+srv://` URL is not accepted as input (see Atlas below).

None of the recipes below has been run against the real vendor service. They are derived from reading `internal/extdb` and the vendors' own pricing pages. Free-tier figures were read on 2026-11-01 and change, so recheck them.

One effect applies to every scale-to-zero service. The health probe runs every five minutes by default and opens a real connection, which can keep an idle database awake and use up free compute or command allowances. Set `APP_EXTERNAL_DB_PROBE_INTERVAL` to a longer interval, or `0` to turn it off, for these.

### Neon (Postgres)

- Fields: host `ep-<name>.<region>.aws.neon.tech`, port 5432, the role and database from the Neon console, TLS mode `require`. Neon refuses plain connections.
- Free plan: 1 GB of storage per project. Compute suspends after 5 minutes idle on the Free plan and cannot be turned off, so the first connection after a pause is slower and a probe can report `slow` while it wakes.
- Use the host without `-pooler` for a first test. The pooled host (`-pooler`) is for many short connections.

### Supabase (Postgres)

- Fields: the host, port, user and database from the project's **Connect** page, TLS mode `require`. The pooler user has the form `postgres.<project-ref>`, which the user field accepts.
- Free plan: 500 MB database size, 2 active projects, and projects are paused after 1 week of inactivity. Whether a probe counts as activity is not verified.
- Whether your network can reach the direct host over IPv4 depends on the plan. Use the pooler host if the test says `unreachable`.

### Upstash (Redis)

- Fields: engine `redis`, host `<name>.upstash.io`, port 6379 (use the port shown in the console), user `default`, the password from the console, TLS mode `require`. Your app then receives a `rediss://` URL.
- Free tier: 250 MB of data and 500K commands per month. A probe every five minutes is a small share of that, but turn it down if you are close.
- The probe passes `--tls --insecure` to `redis-cli`, so it does not verify the server certificate. Your own app should.

### MongoDB Atlas

- `mongodb+srv://` URLs are not supported. The SRV form needs a DNS lookup of the cluster's seed list, and Levelrail stores one plain hostname and port and probes with `mongosh --host --port`. A replica set seed list is not supported either.
- A single shard hostname (`<cluster>-shard-00-00.<id>.mongodb.net:27017`) passes the host checks, but it may point at a secondary and not the primary. Whether it works end to end has not been verified. Prefer to keep Atlas outside Levelrail and put its URL straight into the app's environment (see [Environment and secrets](/environment-and-secrets)).
- Atlas also needs your server's IP on its network access list. Free tier: M0, 512 MB of storage.

### Turso (libSQL)

- Not supported. Turso speaks libSQL over HTTP and WebSocket, not one of the five engines (Postgres, MySQL, MariaDB, MongoDB, Redis), so it cannot be added as an external database. Put its URL and token in the app's environment instead.

## URL and TLS mode by engine

The TLS mode is one of `disable`, `prefer` or `require`, for any engine. The default is `prefer` for Postgres, MySQL and MariaDB, and `disable` for MongoDB and Redis. The app URL is built from the stored host, port, user, database and TLS mode, plus the password from the encrypted store, when the container is created. The table shows the URL shape. Placeholders are in capitals.

| Engine | App URL (`url` field) | What `require` changes in the URL |
| --- | --- | --- |
| postgres | `postgres://USER:PASSWORD@HOST:PORT/DATABASE?sslmode=MODE` | `sslmode=require`. The parameter is always present, because the mode always has a value. |
| mysql | `mysql://USER:PASSWORD@HOST:PORT/DATABASE` | Nothing. The URL has no TLS parameter. The mode only drives the health probe. |
| mariadb | `mariadb://USER:PASSWORD@HOST:PORT/DATABASE` | Nothing, as for mysql. |
| mongodb | `mongodb://USER:PASSWORD@HOST:PORT/DATABASE` | `?tls=true`. The URL has no `authSource`, so a user created in another database needs that parameter added in the app. |
| redis | `redis://USER:PASSWORD@HOST:PORT` | The scheme becomes `rediss://`. No database path is added. |

For `redis`, the user part is present only when a password is stored. For `mysql` and `mariadb`, the URL carries no TLS setting, so the app's driver options must ask for TLS (pending run).

The health probe does not verify the server certificate when the mode is `require`. An app should verify the certificate itself.

### Per provider: the fields to enter

Every row is untested. Each one is a starting point to check with **Test connection**, not a verified recipe (pending run).

| Provider | Engine | Host and port | TLS mode to try first | App URL shape |
| --- | --- | --- | --- | --- |
| Neon | postgres | the host from the Neon console, without `-pooler` for a first test, port 5432 | `require` | `postgres://USER:PASSWORD@HOST:5432/DATABASE?sslmode=require` |
| Supabase | postgres | the host and port from the project's Connect page | `require` | `postgres://USER:PASSWORD@HOST:PORT/DATABASE?sslmode=require` |
| Upstash | redis | `<name>.upstash.io` and the port shown in the console | `require` | `rediss://default:PASSWORD@HOST:PORT` |
| MongoDB Atlas | mongodb | one shard hostname and port 27017 (SRV URLs are not accepted) | `require` | `mongodb://USER:PASSWORD@HOST:27017/DATABASE?tls=true` |
| Self-hosted Postgres on a LAN | postgres | the server IP and port 5432 | `prefer`, then `require` if the server offers TLS | `postgres://USER:PASSWORD@IP:5432/DATABASE?sslmode=prefer` |
| Turso | none | not supported | not applicable | not applicable |

Set the mode in the form, then click **Test connection**. A `tls_error` result means the mode does not match what the server offers. Change it, test again, and keep the mode that passes.

## Not covered yet

- Scheduled backups of an external database. The backup pipeline dumps by
  running the engine's tool inside the database's own container; reusing it
  for a remote host needs a dump source that is not tied to a managed
  container.
- The key browser and SQL console for MongoDB and Redis.
- Adopting from a remote agent node lists containers but not their Docker
  networks, so enter the network by hand there.

## Troubleshooting

### Test connection fails with "unreachable" or "connection refused"

Likely cause: The host and port are wrong, the database is not running, or a firewall is blocking the path.

How to confirm: Try connecting manually from the control plane node: `psql -h <host> -p <port> -U <user> <database>` for Postgres, or `mysql -h <host> -p <port> -u <user> -e "SELECT 1" <database>`. If it hangs or refuses, the network path is blocked.

Fix: Verify the host IP or hostname is correct. Check that the database is running on the remote machine. If behind a firewall, ensure the port is open from the control plane node to the database host. Run the test again: `levelrail-cli databases probe <name>` or Dashboard > Databases > <database> > Check now.

Still stuck: Run `curl -v <host>:<port>` from the control plane to diagnose network connectivity.

### Test connection fails with "auth_failed" or "permission denied"

Likely cause: The username or password is wrong, or the database server has access control rules that deny the connection.

How to confirm: Test manually with the given credentials: `psql -h <host> -p <port> -U <user> -d <database> -c "SELECT 1"` and provide the password. If it fails, the credentials are wrong. For Postgres, check the server's `pg_hba.conf` file to see what auth is required.

Fix: Verify the username and password match what is configured on the database server. For Postgres, ensure `pg_hba.conf` allows connections from the control plane IP for that user and database. Run the test again once the credentials are corrected.

Still stuck: See the database server logs (Postgres: `psql ... -c "SELECT pg_read_file('postgresql.log')"`) for authentication errors.

### Health check shows "slow" or times out

Likely cause: The network is congested, the database is overloaded, or the `APP_EXTERNAL_DB_SLOW_MS` threshold is too strict.

How to confirm: Check the response time when running the health probe manually: `levelrail-cli databases probe <name> --json` shows the query latency. Run it a few times to see if it is consistent. Check the database and network load on both the control plane and database sides.

Fix: If the database is genuinely slow due to load or queries, consider optimizing queries or adding resources. If the probe threshold is too strict, increase `APP_EXTERNAL_DB_SLOW_MS` (default 1500 ms) on the control plane. Apps using the database are not affected by slow health checks, only monitoring.

Still stuck: Monitor the database performance and connection pool to identify bottlenecks.

### Adopted container not reachable after restart

Likely cause: The container was recreated with a different network or IP, or moved to a different node.

How to confirm: Check the container's current network: `docker container inspect <container> | grep NetworkID`. Compare it to what Levelrail has on file: `levelrail-cli databases get <name> --json | jq .docker_network`. If they differ, the container moved or the network changed.

Fix: The record follows the container when recreated on the same node, because it uses the container name. If the container moved to a different node or network, re-adopt it: `levelrail-cli databases adopt --container <new-container> --name <same-name>` (the name must match to avoid duplicates). Or update the external database manually: `levelrail-cli databases connect --name <name> --host <new-host> --test --password-stdin`.

Still stuck: See [Adopt a running container](/external-databases#adopt-a-running-container).

### Cannot write to external database, only read works

Likely cause: The external database record uses a read-only user, or write access is not granted to that user role.

How to confirm: Check what user is configured: `levelrail-cli databases get <name> --json | jq .user`. Try a write manually: `psql -h <host> -U <user> -c "INSERT INTO test VALUES (1)"`. If it fails with permission denied, the user lacks write permissions.

Fix: Either create a new external database record with a write-enabled user, or grant the user write permissions on the database server. For Postgres: `ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT INSERT, UPDATE, DELETE ON TABLES TO <user>`.

Still stuck: Use the database viewer (Databases > <database> > Editor) with an admin user instead, or reconfigure the external database with a different user role that has write access.
