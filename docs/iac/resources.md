---
title: "IaC resource kinds"
tag: "Experimental"
description: Each resource kind with a worked example and the rules that do not fit a field table. The field list itself is the generated reference.
---

The field tables are in the [IaC reference](/iac/reference), generated from the schema. This page keeps one worked example per kind and the rules around it.

## Document format

A file holds one or more YAML documents separated by `---`. Every document has the same envelope:

```yaml
version: 1              # schema version, the only accepted value
kind: App               # Project, Environment, Tag, Database, App, Domain, LoadBalancer, Pipeline, AlertRule
metadata:
  name: web             # unique per kind (see each kind below)
  labels: {}            # free form, for your own tooling; not stored
spec: {}                # kind specific
```

The [JSON Schema](https://github.com/glincker/levelrail/blob/main/docs/schemas/resources.schema.json) is also served at `GET /api/v1/apply/schema`. Point your editor at it for completion. Errors are reported with file and line: `infra/web.yaml:14: additional properties 'prot' not allowed (spec.service.prot)`.

An App's `spec.service` is the app.yaml service definition, validated by the same schema. The document schema embeds the app.yaml definitions unchanged, with two additions: an env value may be `{ secretRef: NAME }`, or `{ from: { database: NAME, field: url } }` to read a value from a Database (see [Database references](#database-references)).

### Project

```yaml
version: 1
kind: Project
metadata: { name: shop }
spec:
  env: { REGION: eu }     # shared, non secret variables
```

### Environment

Environments belong to a project. Identity is project plus name.

```yaml
version: 1
kind: Environment
metadata: { name: production }
spec:
  project: shop
  protected: true
  env: { LOG_LEVEL: info }
```

### Tag

```yaml
version: 1
kind: Tag
metadata: { name: frontend }
```

Export prints every tag, including tags no app uses.

### Database

```yaml
version: 1
kind: Database
metadata: { name: main-db }
spec:
  engine: postgres
  version: "16"
  project: shop
```

Engine and version cannot change through apply: databases hold data, so a mismatch is reported as an error and you recreate it deliberately. Backups, resources and public access are still managed with their own commands.

### App

```yaml
version: 1
kind: App
metadata: { name: web }
spec:
  project: shop
  environment: production
  tags: [frontend]
  service:
    build: { type: image, image: "ghcr.io/acme/web:1.2.3" }
    port: 3000
    domains: [app.example.com]
    replicas: 2
    strategy: blue-green
    resources: { memory: 512Mi, cpu: 0.5 }
    health:
      readiness: { path: /healthz, interval: 5s, timeout: 2s }
    env:
      LOG_LEVEL: info
      API_KEY: { secretRef: API_KEY }
      DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
      DATABASE_URL: { from: { database: main-db, field: url } }
    volumes:
      - name: data
        path: /data
    jobs:
      - name: nightly-report
        schedule: "0 3 * * *"
        command: [node, scripts/report.js]
        timeout: 10m
```

Apply manages apps that run a prebuilt image (`build.type: image`). Git builds are configured with a git source, not here. `port`, `health.readiness` and `health.liveness` are applied as written. Not supported yet, and rejected with a clear error: host-path volumes, egress policies, GPU requests, registry credentials and `vault` env references. A live app that uses them keeps them when you apply; the plan warns.

#### Database references

`{ from: { database: NAME, field: FIELD } }` points an env var at a Database's connection details. `field` is one of `url` (the default), `host`, `port`, `username`, `password` or `database`. The app.yaml string form `{ from: main-db.url }` is accepted too.

The value is never read by apply. Apply records the reference (the same connection the dashboard's Connect button makes), and the control plane fills in the value when it creates the container. So the files, the plan, the plan hash and stack revisions only ever hold `main-db.url`, never the password or the URL. The database must exist or be declared in the same files; the plan shows the reference as `connections.DATABASE_URL: main-db.url` and lists the database under `dependsOn`, and databases are always applied before apps. If the live app also has a literal or secret value under the same name, the plan warns so you can remove it.

#### Volumes

`service.volumes` lists named volumes (`name`, `path`). Apply matches them by name through `PUT /api/v1/apps/{name}/volumes`: a new name is attached, a changed path is updated, and the plan shows `volumes.data: /data`. Host-path bind mounts are rejected, so the files can never mount a host directory. Volumes that the files do not list are kept, and detached only by prune on an app the source manages. Detaching never deletes the Docker volume or its data; deleting data is a separate, explicit action. Mounts are fixed when the container is created, so a volume change restarts the app unless `--no-deploy` is set, and the app shows a pending config change until then. An app document without a `volumes` key never touches the app's volumes.

#### Published ports and restart

`service.published_ports` binds ports straight on the host beside the routed `port`, for UDP services such as DNS or WireGuard. Each entry is `container_port`, `host_port` and `protocol` (`tcp`, the default, or `udp`), the same shape as app.yaml and the API. A port out of range, a protocol other than tcp or udp, or the same host port and protocol twice is rejected before the plan. Apply runs the same host port admission as `POST /api/v1/apps`: a port another app already publishes, a reserved port, or a service placed on a remote node fails the apply with the API's own message, and nothing is saved for that app.

`service.port` may be left out when the service has at least one `published_ports` entry (a UDP-only DNS or VPN app). A service with `domains` still needs a `port`, since the proxy routes HTTP to it, and a service with neither a `port` nor `published_ports` is rejected.

`service.restart` takes `no`, `on-failure` or `on-failure:N` for a service that runs to completion (a migration or seed job), and `always` or `unless-stopped`. The plan shows changes as `publishedPorts` and `restart`, an out-of-band change to either is drift, and revisions and rollbacks keep both. An app document without `published_ports` or `restart` leaves the live values alone; `published_ports: []` clears the ports.

```yaml
version: 1
kind: App
metadata: {name: dns}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/dns:1.0.0"}
    port: 8080
    published_ports:
      - {container_port: 53, host_port: 53, protocol: udp}
      - {container_port: 53, host_port: 53, protocol: tcp}
```

#### Rollout policy

`service.rollout` sets `max_surge`, `max_unavailable` and `min_available` (each an integer such as `2` or a percent such as `25%`). Apply saves it through `PUT /api/v1/apps/{name}/rollout`, the same policy a spec deploy and `levelrail-cli rollout status` read, and export prints it back. The plan shows `rollout.min_available: 1 -> 2`. Fields left out inherit the `APP_ROLLOUT_*` env defaults. An app document without a `rollout` key never touches the saved policy; `DELETE /api/v1/apps/{name}/rollout` returns an app to the defaults.

#### Inline files

`service.files` lists text files written into the container (`path`, `content`, optional `mode`). Apply sets each one through `PUT /api/v1/apps/{name}/files` and removes live files the list omits, so a declared list owns the app's non-secret files; export prints them back. Secret files are rejected in documents, because their content would sit in git: set them from the dashboard or CLI, and apply leaves them alone. Files are fixed at container create, so a change restarts the app unless `--no-deploy` is set. An app document without `files` never touches them.

Pre-deploy snapshot policy, the CI gate of a git source and the heartbeat monitor are instance or git-source settings, not part of an App document yet; manage them from the dashboard, CLI or API.

#### Scheduled jobs

`service.jobs` is the app.yaml jobs list. Apply matches jobs by name: a new name is created, a changed one is updated in place (its run history is kept). Jobs created in the dashboard or CLI that the files do not list are kept, and removed only by prune on an app the source manages. A job that the app's own app.yaml declares is left alone with a warning, so each job has one owner. An app document without a `jobs` key never touches the app's jobs.

### Domain

Adds a hostname to an app. It is additive with the App's own `domains`, and a hostname can belong to one app only. Export folds domains into each App's `service.domains` and does not print Domain documents.

```yaml
version: 1
kind: Domain
metadata: { name: www.example.com }
spec: { app: web }
```

### LoadBalancer

`metadata.name` is the app. The spec is the app.yaml `loadbalancer` block.

```yaml
version: 1
kind: LoadBalancer
metadata: { name: web }
spec: { algorithm: least_conn }
```

### Pipeline

```yaml
version: 1
kind: Pipeline
metadata: { name: ci }
spec:
  app: web
  enabled: true
  yaml: |
    version: 1
    jobs:
      test:
        image: golang:1.22
        steps:
          - run: go test ./...
```

The pipeline definition is validated with the same checks as `pipelines validate`, and issues point at the line inside your file. A pipeline synced from the repository cannot be changed here.

### AlertRule

```yaml
version: 1
kind: AlertRule
metadata: { name: high-cpu }
spec:
  app: web
  type: threshold
  metric: cpu_percent
  comparator: ">"
  threshold: 90
  for: 5m
  channel: ops          # a notification channel, by name
```
