---
title: "External databases"
description: Connect a database that already runs somewhere else, or adopt a running container, without moving or changing any data, and point apps at it.
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
levelrail-cli databases adopt --container coolify-pg --name legacy \
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
