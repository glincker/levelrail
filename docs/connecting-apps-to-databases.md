---
description: Connect an app to one or more managed databases, read the same-node/cross-node/mesh-disabled reachability badges, and see how this differs from the older single-attachment flow.
---

# Connecting apps to databases

An app can attach to exactly one database with the older `PUT /api/v1/apps/{name}/database` flow (see [Managing databases](managing-databases.md#attaching-a-database-to-an-app)). Connections are the newer, more general mechanism: an app can connect to as many databases as it needs, each one landing in its own env var.

Both mechanisms coexist and write to different fields on the same app record, so an app can have one legacy attachment and several connections at the same time. Connections are the better default for new work since they are not capped at one database.

## Connecting

::: code-group

```bash [CLI]
levelrail-cli apps connect your-app main-db
levelrail-cli apps connect your-app cache --field host --env-var CACHE_HOST
```

```text [Dashboard]
App detail page -> "Connections" card -> "Connect a database"
```

:::

`field` defaults to `url` (the full connection string). The other resolvable fields are `host`, `port`, `username`, `password`, and `database`, validated per engine the same way the older attachment flow validates them (Redis, KeyDB, and Dragonfly don't support `username` or `database`).

`env_var` defaults to `<DATABASE>_DATABASE_URL` for the `url` field, or `<DATABASE>_DB_<FIELD>` otherwise, with a numeric suffix appended if that name is already taken by another env var, secret, vault ref, connection, or the legacy attachment. Pass `--env-var` to pick your own name. Connecting again with the same `env_var` replaces that entry.

Not sure which databases make sense for an app? `levelrail-cli apps connections suggest your-app` lists every database the app could connect to (scoped to the app's project when it has one), flagging which ones are already connected. It's a listing, not compatibility matching: it doesn't check engine or schema fit, just what exists.

## Reading the reachability badge

Every connection shows one of three badges, both in `apps connections list` and on the dashboard's Connections card:

- **Same node**: the app and the database run on the same node. Resolves to a plain Docker container name.
- **Cross-node (mesh DNS)**: the app and database are on different nodes, and this control plane has WireGuard mesh networking configured. Resolves to a mesh DNS name that reaches the database regardless of which node it's on.
- **Cross-node, mesh disabled**: the app and database are on different nodes, but mesh networking isn't configured on this control plane. This connection will not reach the database. Enable mesh networking, or move the app or the database onto the same node.

The badge is a straight comparison of the app's and database's node IDs, plus whether mesh networking is configured at all. It updates whenever either resource moves.

## System-managed env vars

Every connection's env var is labeled **System-managed** on the dashboard. It's injected by the reconciler at container start, not read from your `app.yaml`'s `env:` block, so hand-editing the same key in the Env tab gets silently overwritten at the next deploy. If you need the value elsewhere (for example baked into a build), read it from the connection rather than duplicating it as a plain env var.

## Disconnecting

```bash
levelrail-cli apps disconnect your-app CACHE_HOST
```

Disconnecting is idempotent: removing an env var that isn't connected is not an error. Like the legacy attachment's detach, it stops injecting the value into newly created containers without touching containers already running.

## See also

- [API reference](api-reference.md) - full request/response shape for `/apps/{name}/connections` and `/apps/{name}/connectable-databases`
- [CLI reference](cli-reference.md) - `apps connect`, `apps disconnect`, `apps connections list`, `apps connections suggest`
- [Managing databases](managing-databases.md#attaching-a-database-to-an-app) - the older single-attachment flow
- [Network topology](network-topology.md) - see every app-to-database connection drawn across the whole mesh at once
