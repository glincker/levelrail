---
description: Seven curated starter-kit templates demonstrating real multi-service Compose patterns, including depends_on start ordering
---

# Starter kit templates

Seven curated, tested multi-service templates in the `Starter Kits` category of the [service template catalog](/templates-and-registry). Unlike the rest of the catalog, which deploys one well-known open-source project per entry, these demonstrate common **wiring patterns** across two or three services: a web tier talking to a database, a worker pulling off a queue, a reverse proxy fanning out to backends. Swap the `web`/`api`/`worker` service's `image:` for your own build output; the environment variable names and `depends_on:` graph are the part meant to be copied as-is.

![Levelrail service templates page with category filters and the Starter Kits cards at the top](assets/screenshots/templates-catalog.png)

See the [full template catalog](/template-catalog) for every other category (AI, Monitoring, Productivity, and the rest).

Every template's Compose body is parsed, validated, and expanded through `internal/compose.Parse`/`Validate`/`ToDesiredServices` in `internal/catalog/catalog_test.go`, the same check every catalog entry gets. They have not been deployed against real Docker as part of this change; only structural/schema validation has run.

**Package:** `internal/catalog/templates_starterkits.go` &middot; served via `GET /api/v1/service-templates` (see [Templates and registry](/templates-and-registry) for the API/CLI/UI surface shared by the whole catalog).

## The seven templates

| ID | What it demonstrates | Env vars |
| --- | --- | --- |
| `node-postgres-starter` | A Node web service depending on a healthchecked Postgres, the most common backend pairing | `DATABASE_URL` (auto-generated password via `$SERVICE_PASSWORD_DB`) |
| `redis-cache-starter` | A web service backed by a Redis cache (cache-aside pattern) | `REDIS_URL` |
| `nextjs-postgres-starter` | The SSR-plus-database pairing behind most Next.js apps | `DATABASE_URL` (auto-generated) |
| `static-site-api-starter` | A static frontend and a separate API service deployed side by side | none required |
| `fastapi-postgres-starter` | A Python HTTP service wired to Postgres, the reference pairing for a FastAPI backend | `DATABASE_URL` (auto-generated) |
| `worker-redis-queue-starter` | A background worker depending on a Redis queue, no HTTP port | `QUEUE_URL` |
| `nginx-multi-backend-starter` | One Nginx entrypoint routing to two independent backends, three services with `depends_on` | none required |

`$SERVICE_PASSWORD_*` tokens auto-generate a real secret at deploy time (see [Templates and registry](/templates-and-registry)'s "Magic variables" section); they are not literal credentials in the source.

## depends_on start ordering

Every template that has a real dependency (`node-postgres-starter`, `nextjs-postgres-starter`, `fastapi-postgres-starter`, `redis-cache-starter`, `worker-redis-queue-starter`, `nginx-multi-backend-starter`) declares `depends_on:` against it. The reconciler (`internal/reconcile/application`) waits for a dependency's container to start before creating the dependent service's own container, real Compose's `service_started` semantic, not `service_healthy`; a `condition:` key would be accepted but silently has no additional effect. `nginx-multi-backend-starter` is the one built specifically to exercise two-way fan-out: the `proxy` service depends on both `orders` and `payments`.

## Deploying one

From the dashboard: **Apps → New app → Browse templates**, filter or search for "Starter Kits", pick a template, review the pre-filled Compose body, deploy.

From the CLI:

```sh
levelrail templates list
levelrail templates get node-postgres-starter
levelrail templates deploy node-postgres-starter --name my-app
```

`templates deploy` is a thin wrapper around the same `POST /api/v1/apps/{name}/compose` call `apps deploy-compose` already makes.

## Not verified

These templates were validated structurally (Compose parse, schema validation, `ToDesiredServices` expansion, dependency-cycle check, healthcheck presence) but were not run end-to-end against a live Docker daemon as part of this change. The reference `web`/`api`/`worker` services intentionally use only their base image's own built-in runtime (Node's `http` module, Python's `http.server`) rather than an installed framework, so there is no `npm install`/`pip install` step to fail at container start, but that has not been confirmed by an actual deploy.
