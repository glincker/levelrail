---
title: "IaC templates"
tag: "Experimental"
description: Copy a working Infrastructure as code starting point, one file per scenario, each checked against the schema in CI.
---

Every file below lives in the repository under `examples/iac/` and is validated by the same parser that runs `levelrail-cli apply`. Copy one, change the names, then run `levelrail-cli diff -f infra/`. Set `APP_EXPERIMENTAL=iac` first, see the [quickstart](/iac/index).

| Template | Use it for | Files |
| --- | --- | --- |
| [Single app](#single-app) | One image behind a port | `examples/iac/single-app/app.yaml` |
| [App and database](#app-and-database) | An app that reads a managed Postgres by reference | `examples/iac/app-database/app.yaml` |
| [Multiple environments](#multiple-environments) | Staging and production from one file and two values files | `examples/iac/multi-env/` |
| [Local development](#local-development) | A throwaway project on a local control plane | `examples/iac/local-dev/` |

## Single app

```yaml
version: 1
kind: Project
metadata: { name: demo }
---
version: 1
kind: App
metadata: { name: web }
spec:
  project: demo
  service:
    build: { type: image, image: "nginx:1.27" }
    port: 80
    replicas: 1
```

## App and database

The app gets its connection string by reference, so no password is written to the file.

```yaml
version: 1
kind: Project
metadata: { name: demo }
---
version: 1
kind: Database
metadata: { name: main-db }
spec:
  engine: postgres
  version: "16"
  project: demo
---
version: 1
kind: App
metadata: { name: api }
spec:
  project: demo
  service:
    build: { type: image, image: "ghcr.io/acme/api:1.0.0" }
    port: 3000
    env:
      DATABASE_URL: { from: { database: main-db, field: url } }
```

## Multiple environments

One manifest, one values file per environment. Placeholders are filled by `--values`, see [how apply works](/iac/how-it-works#values-per-environment). Files: `app.yaml`, `values/staging.yml`, `values/prod.yml`.

```yaml
# examples/iac/multi-env/app.yaml
version: 1
kind: Project
metadata: { name: shop }
---
version: 1
kind: Database
metadata: { name: shop-db }
spec: { engine: postgres, version: "16", project: shop }
---
version: 1
kind: App
metadata: { name: shop-web }
spec:
  project: shop
  service:
    build: { type: image, image: "ghcr.io/acme/shop:${{ env.IMAGE_TAG }}" }
    port: 3000
    replicas: "${{ env.REPLICAS }}"
    domains: ["${{ env.DOMAIN }}"]
    env:
      LOG_LEVEL: ${{ env.LOG_LEVEL }}
      DATABASE_URL: { from: { database: shop-db, field: url } }
```

```yaml
# examples/iac/multi-env/values/prod.yml
IMAGE_TAG: 1.8.2
REPLICAS: 2
DOMAIN: shop.example.com
LOG_LEVEL: info
```

```bash
levelrail-cli diff -f examples/iac/multi-env/app.yaml --values examples/iac/multi-env/values/staging.yml
levelrail-cli apply -f examples/iac/multi-env/app.yaml --values examples/iac/multi-env/values/prod.yml --yes
```

## Local development

A small project for a local control plane. Files: `app.yaml`, `.env.example` and a `Makefile` with `plan` and `apply` targets.

```yaml
# examples/iac/local-dev/app.yaml
version: 1
kind: Project
metadata: { name: sandbox }
---
version: 1
kind: Database
metadata: { name: dev-db }
spec: { engine: postgres, version: "16", project: sandbox }
---
version: 1
kind: App
metadata: { name: dev-api }
spec:
  project: sandbox
  service:
    build: { type: image, image: "ghcr.io/acme/api:latest" }
    port: 3000
    replicas: 1
    env:
      LOG_LEVEL: ${{ env.LOG_LEVEL }}
      DATABASE_URL: { from: { database: dev-db, field: url } }
```

```bash
cd examples/iac/local-dev
cp .env.example .env.local
make plan
make apply
```

Next: [errors](/iac/errors), [IaC reference](/iac/reference).
