---
description: Connect a database that already runs somewhere else, or adopt a running container, without moving or changing any data, and point apps at it.
---

# External databases

An external database is one Levelrail connects to but does not run. Use it when
a database already holds real data and you want apps to use it where it is:
a Postgres container left over from another platform, a managed cloud
database, or a server on your LAN. Nothing is copied, restarted or modified.
You can move the data into a managed database later, or never.

## What it does and does not do

- Stores the address, user, database name, TLS mode and Docker network. The
  password goes into the same envelope-encrypted secrets store as every other
  credential and is never returned by the API.
- Shows the database in the Databases list with an **External** badge. Actions
  that only make sense for a database Levelrail runs (stop, start, resources,
  version change, major upgrade, backups, public access) are not offered.
- Injects the same connection env into apps as a managed database does
  (`host`, `port`, `username`, `password`, `database`, `url`).
- Opens the [database viewer](database-viewer.md) against it (Postgres, MySQL,
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
