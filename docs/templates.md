---
description: Seven starter-kit templates that show multi-service Compose wiring patterns: web plus database, worker plus queue, reverse proxy plus backends.
---

# Starter kit templates

The `Starter Kits` category of the [template catalog](/template-catalog) holds seven small multi-service templates. Unlike the rest of the catalog, where each entry deploys one open-source project, these show wiring patterns across two or three services: a web tier talking to a database, a worker pulling from a queue, a reverse proxy fanning out to backends.

Swap the `web`, `api` or `worker` service's `image:` for your own build output. The environment variable names and the `depends_on:` graph are the part meant to be copied as is.

![Levelrail service templates page with category filters and the Starter Kits cards at the top](assets/screenshots/templates-catalog.png)

How to deploy any template, and how `$SERVICE_...` variables work, is covered in [Templates and registry](/templates-and-registry).

## The seven templates

| ID | What it demonstrates | Env vars |
| --- | --- | --- |
| `node-postgres-starter` | A Node web service depending on a healthchecked Postgres | `DATABASE_URL` (password generated via `$SERVICE_PASSWORD_DB`) |
| `redis-cache-starter` | A web service backed by a Redis cache (cache-aside) | `REDIS_URL` |
| `nextjs-postgres-starter` | The server-rendered-app-plus-database pairing behind most Next.js apps | `DATABASE_URL` (generated) |
| `static-site-api-starter` | A static frontend and a separate API service side by side | none required |
| `fastapi-postgres-starter` | A Python HTTP service wired to Postgres, the usual FastAPI backend pairing | `DATABASE_URL` (generated) |
| `worker-redis-queue-starter` | A background worker depending on a Redis queue, no HTTP port | `QUEUE_URL` |
| `nginx-multi-backend-starter` | One Nginx entrypoint routing to two independent backends | none required |

`$SERVICE_PASSWORD_*` tokens generate a real secret at deploy time, so no credential is stored in the template itself.

## Start ordering with depends_on

Every template with a real dependency declares `depends_on:` on it. The reconciler waits until the dependency's container is running before it creates the dependent service's container. This is Compose's `service_started` behavior: it orders startup but does not wait for the dependency to be healthy, and a `condition:` key is accepted with no additional effect. In `nginx-multi-backend-starter` the `proxy` service depends on both `orders` and `payments`.

## Deploy one

<Tabs :items="['Dashboard', 'CLI']">
<Tab value="Dashboard">

Go to **Apps**, choose **New app**, then **Browse templates**. Search for "Starter Kits", pick a template, review the pre-filled Compose body, and deploy.

</Tab>
<Tab value="CLI">

```sh
levelrail-cli templates list
levelrail-cli templates get node-postgres-starter
levelrail-cli templates deploy node-postgres-starter --name my-app
```

</Tab>
</Tabs>

## Caveats

Each template's Compose body is checked structurally in the test suite (parse, validation, expansion into services, dependency cycles, healthcheck presence). They are reference wiring, not production-ready apps: the `web`, `api` and `worker` services run a minimal server built from their base image's own runtime (Node's `http` module, Python's `http.server`) so that nothing needs installing at start. Replace them with your own image.
