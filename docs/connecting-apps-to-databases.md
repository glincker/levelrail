---
description: Connect an app to one or more managed databases, read the same-node, cross-node and mesh-disabled reachability badges, and see how connections differ from the older single-attachment flow.
---

# Connecting apps to databases

An app can attach to exactly one database with the older `PUT /api/v1/apps/{name}/database` flow (see [Managing databases](managing-databases.md#attach-a-database-to-an-app)). Connections are the newer, more general mechanism: an app can connect to as many databases as it needs, each one landing in its own env var.

Both mechanisms coexist and write to different fields on the same app record, so an app can have one legacy attachment and several connections at the same time. Use connections for new work, since they are not capped at one database.

## Connect a database

<Tabs :items="['CLI', 'Dashboard']">
<Tab value="CLI">

```bash
levelrail-cli apps connect your-app main-db
levelrail-cli apps connect your-app cache --field host --env-var CACHE_HOST
```

</Tab>
<Tab value="Dashboard">

Open the app's detail page, find the **Connections** card, and choose **Connect a database**.

</Tab>
</Tabs>

`field` defaults to `url` (the full connection string). The other resolvable fields are `host`, `port`, `username`, `password`, and `database`. They are validated per engine the same way the older attachment flow validates them: Redis, KeyDB, and Dragonfly do not support `username` or `database`.

`env_var` defaults to `<DATABASE>_DATABASE_URL` for the `url` field, or `<DATABASE>_DB_<FIELD>` otherwise (for example `MAIN_DB_HOST`). A numeric suffix is appended if that name is already taken by another env var, secret, vault ref, connection, or the legacy attachment. Pass `--env-var` to pick your own name. Connecting again with the same `env_var` replaces that entry.

To see which databases an app could connect to, run `levelrail-cli apps connections suggest your-app`. It lists every database the app could connect to (scoped to the app's project when it has one) and flags the ones already connected. It does not check engine or schema fit. `levelrail-cli apps connections list your-app` shows the current connections.

## Reachability badges

Every connection shows one of three badges, in `apps connections list` and on the dashboard's Connections card.

| Badge | Meaning |
| --- | --- |
| **Same node** | The app and the database run on the same node. The name resolves to a plain Docker container name. |
| **Cross-node (mesh DNS)** | They run on different nodes and this control plane has WireGuard mesh networking configured. The name resolves to a mesh DNS name that reaches the database wherever it runs. |
| **Cross-node, mesh disabled** | They run on different nodes but mesh networking is not configured. This connection will not reach the database. Enable mesh networking, or move the app or the database onto the same node. |

The badge compares the app's and the database's node IDs and checks whether mesh networking is configured at all. It updates whenever either resource moves.

## System-managed env vars

The dashboard labels every connection's env var **System-managed**. The reconciler injects it at container start. It is not read from your `app.yaml` `env:` block, so editing the same key in the Env tab is overwritten at the next deploy. If you need the value elsewhere (for example in a build), read it from the connection instead of duplicating it as a plain env var.

## Disconnect

```bash
levelrail-cli apps disconnect your-app CACHE_HOST
```

Disconnecting is idempotent: removing an env var that is not connected is not an error. Like the legacy attachment's detach, it stops injecting the value into newly created containers and does not touch containers that are already running.

## Next steps

<CardGroup :cols="2">
<Card title="Managing databases" href="/managing-databases">

Create, back up, and restore the databases you connect to.

</Card>
<Card title="Network topology" href="/network-topology">

See every app-to-database connection drawn across the mesh.

</Card>
<Card title="API reference" href="/api-reference">

Request and response shapes for `/apps/{name}/connections` and `/apps/{name}/connectable-databases`.

</Card>
<Card title="CLI reference" href="/cli-reference">

Every `apps connect`, `apps disconnect`, and `apps connections` flag.

</Card>
</CardGroup>
