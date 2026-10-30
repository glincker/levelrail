---
title: "Starter kit templates"
description: "Seven starter-kit templates that show multi-service Compose wiring patterns: web plus database, worker plus queue, reverse proxy plus backends."
"og:url": "https://docs.levelrail.com/templates"
---

The `Starter Kits` category of the [template catalog](/template-catalog) holds seven small multi-service templates. Unlike the rest of the catalog, where each entry deploys one open-source project, these show wiring patterns across two or three services: a web tier talking to a database, a worker pulling from a queue, a reverse proxy fanning out to backends.

Swap the `web`, `api` or `worker` service's `image:` for your own build output. The environment variable names and the `depends_on:` graph are the part meant to be copied as is.

<img src="/assets/screenshots/templates-catalog-light.png" alt="Levelrail service templates page with category filters and the Starter Kits cards at the top" className="block dark:hidden" />
<img src="/assets/screenshots/templates-catalog.png" alt="Levelrail service templates page with category filters and the Starter Kits cards at the top" className="hidden dark:block" />

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

<Tabs>
<Tab title="Dashboard">

Go to **Apps**, choose **New app**, then **Browse templates**. Search for "Starter Kits", pick a template, review the pre-filled Compose body, and deploy.

</Tab>
<Tab title="CLI">

```sh
levelrail-cli templates list
levelrail-cli templates get node-postgres-starter
levelrail-cli templates deploy node-postgres-starter --name my-app
```

</Tab>
</Tabs>

## Access details

An app deployed from a template records its template id and returns it as `template_id` on `GET /api/v1/apps/{name}`. Its app page then shows an **Access details** card on the overview, and the app header menu has **Open access details**, which opens the same content in a dialog. Apps without a recorded id show it too when they carry generated `SERVICE_PASSWORD_*`, `SERVICE_USER_*` or `SERVICE_FQDN_*` keys or are trials.

The card shows the app address, the generated credentials, and the template's setup notes. Credential values stay masked until you press reveal. Reveal follows the app's environment policy: it is unavailable when reveal is disabled or secrets are write only, may ask for step-up confirmation, and a denied reveal shows an error. The same flow is the app's environment page, or `levelrail-cli apps env get <app> <KEY> --reveal` with the `secrets:reveal` ability. The browse templates dialog shows the same result after a deploy.

## Caveats

Each template's Compose body is checked structurally in the test suite (parse, validation, expansion into services, dependency cycles, health check presence). Templates are reference wiring, not production-ready apps: the `web`, `api` and `worker` services run a minimal server built from their base image's own runtime (Node's `http` module, Python's `http.server`) so that nothing needs installing at start. Replace them with your own image.

## Shipping a config file with a template

Apps that need a config file at start declare it as a top-level Compose `configs:` entry with inline `content:` and reference it from the service with a `target:` path. The file is written into the container before it starts. `file:` and `external:` sources are not supported. See [App files](/app-files) for limits and editing after install.
