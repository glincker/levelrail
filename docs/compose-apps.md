---
title: "Compose apps"
description: Run a Docker Compose file as one app with per-service status, start order, rollback to an earlier compose file, teardown, and installable templates.
"og:url": "https://docs.levelrail.com/compose-apps"
---

A compose app is one app built from a Docker Compose file. Every service in the file becomes a member service of the app with its own containers, logs, metrics and status. All of them share a private network where they reach each other by service name. The reconciler handles each service on its own: a broken service never blocks a sibling that does not depend on it, and a container you kill by hand comes back on the next pass.

You can deploy a compose app three ways:

- In the dashboard, open **New resource**, pick **Docker Compose**, then paste or upload the file. A preview lists every service, port, domain, volume and env var before anything is deployed.
- From the CLI, run `levelrail-cli compose deploy <app> -f compose.yaml`.
- From a template, open a template page and fill in its settings, or run `levelrail-cli templates install <id>`.

To deploy a compose file that lives in a git repository and needs `build:`, use a git deploy with `build.type: compose` ([Deploying apps](/deploying-apps)). This page covers compose files that use pre-built images.

## Deploy a stack

```yaml
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: ${SERVICE_PASSWORD_DB}
    volumes:
      - pgdata:/var/lib/postgresql/data
  api:
    image: ghcr.io/acme/api:1.4.2
    ports: ["8080"]
    environment:
      DATABASE_URL: postgres://postgres:${SERVICE_PASSWORD_DB}@db:5432/postgres
    depends_on:
      db:
        condition: service_healthy
  web:
    image: ghcr.io/acme/web:1.4.2
    ports: ["3000"]
    depends_on: [api]
x-levelrail-domains:
  web: shop.example.com
```

```bash
levelrail-cli compose preview -f compose.yaml      # check it, nothing is saved
levelrail-cli compose deploy shop -f compose.yaml  # deploy as app "shop"
levelrail-cli compose status shop                  # per-service status
```

Each deploy is saved as a numbered revision. Deploying the same app name again updates the stack in place. Services whose config changed are recreated, and services the new file no longer declares are removed.

### What each key does

| Compose key | Behavior |
| --- | --- |
| `image` | Required. `build:` is rejected here; use a git deploy for that. |
| `ports` | The first container port is the one ingress routes to. Host ports are never published. |
| `x-levelrail-domains` | Maps a service to a domain. HTTPS is issued automatically once DNS points at the server. |
| `volumes` | Named volumes become Docker volumes scoped to the app and service. An absolute host path is a bind mount and needs the `root` ability. |
| `environment` | Literal values are stored as config. `${SERVICE_PASSWORD_X}` and the other generated kinds are created once, encrypted, and injected when the container is created. |
| `depends_on` | A dependent service is created only after its dependency's container runs. With `condition: service_healthy`, it also waits until the dependency passes its readiness check. |
| `healthcheck` | An HTTP check (`curl` or `wget` against localhost) becomes an HTTP readiness probe. Any other command runs inside the container as an exec probe. |
| `profiles` | A service with `profiles:` only deploys when one of its profiles is enabled with `--profile-name` (or `?profiles=` on the API). |
| `deploy.replicas`, `deploy.resources.reservations.devices` | Replica count and GPU reservations. Other Swarm keys are rejected. |
| `restart`, `networks` | Parsed and reported as notices. The reconciler restarts containers, and every service shares one app network. |

### What is rejected

These keys widen a container's access to the host, so a compose file that sets any of them is rejected with a clear error. The rule name in brackets matches the Docker socket guard ([Docker access](/docker-access)):

- `privileged: true` [privileged]
- `network_mode: host` [network_host], and `network_mode: service:...` or `container:...`
- `pid`, `ipc`, `uts`, `userns_mode` or `cgroup` set to `host`
- `cap_add`, `devices`, `volumes_from`, and `security_opt` (except `no-new-privileges`)
- a bind mount of the Docker socket (`docker.sock`, `podman.sock` or `containerd.sock`) from any path, plus system paths such as `/`, `/etc` and `/var/lib/docker`
- top-level and per-service `secrets:` and `configs:`

## Status and per-service control

`levelrail-cli compose status <app>` and the **Compose stack** card on the app's **Services** tab show:

- One phase for the stack:
  - `running`: every service is ready.
  - `progressing`: something is still starting or waiting.
  - `degraded`: at least one service is failing.
  - `failed`: every service is failing.
  - `stopped`
- Each service's ready state with the reconciler's reason, for example `WaitingForDependency`, `WaitingForDependencyHealthy` or `CreateFailed`, plus its message.

Logs and metrics stay per service. Each member is a normal app, named `<app>-<service>`, with all the usual app pages.

```bash
levelrail-cli compose logs shop                    # recent lines from every service, merged
levelrail-cli compose logs shop --service api -f   # follow one service
levelrail-cli compose redeploy shop                # recreate every service
levelrail-cli compose redeploy shop --service api  # recreate one
```

## Rollback

Every deploy records the compose file it used. Rolling back re-applies an earlier file through the normal deploy path and saves the result as a new revision, so you can roll forward again later. Generated passwords are stored per app, not per revision, so an older file still uses the same database password that the data in its volumes expects.

```bash
levelrail-cli compose revisions shop
levelrail-cli compose rollback shop                # back to the revision before the latest
levelrail-cli compose rollback shop --revision 2
```

## Tear down

```bash
levelrail-cli compose down shop            # removes services and containers, keeps volumes
levelrail-cli compose down shop --volumes  # also deletes the stack's named volumes
```

By default, named volumes and generated secrets are kept. Deploying the same app name again picks up the old data. `--volumes` deletes the data for good and needs the `write-sensitive` ability.

## Templates

A template is a directory under `internal/catalog/manifests/<id>/` with a `template.yaml` and a compose file:

```yaml
id: wikijs
name: Wiki.js
description: A modern wiki backed by Postgres.
icon: book
category: Productivity
documentation_url: https://docs.requarks.io
recommended_memory_bytes: 536870912
env:
  - key: DB_PASSWORD
    label: Database password
    generate: password     # password, user, hex or base64
    length: 32
    secret: true
  - key: ADMIN_EMAIL
    required: true         # the installer must supply it
domains:
  - service: wiki
    port: 3000
healthcheck:
  service: wiki
  port: 3000
  path: /healthz
```

The compose file refers to each setting as `${KEY}`. When a template is installed, each setting gets the value the operator supplied, a generated secret (stored encrypted and reused on every redeploy), or the default.

Values are inserted into parsed YAML values, never into the raw text, so a value cannot change the structure of the file. A template whose settings and `${KEY}` uses do not match fails to load. One that fails the compose rules above also fails to load.

```bash
levelrail-cli templates get wikijs
levelrail-cli templates install wikijs --name wiki --domain wiki=wiki.example.com
levelrail-cli templates install redmine --env SECRET_KEY_BASE=... --name tracker
```

The dashboard's template page shows the same form: one field per setting, generated values listed but not asked for, and one optional domain per service.

Five templates ship in this format today: Uptime Kuma, Wiki.js, Redmine, One-Time Secret and PairDrop. Each one is installed on real Docker by the live test suite (`TestComposeTemplates_Live`). The rest of the catalog uses the older single compose body format and still deploys the same way.

### Checking a batch of compose files

`go run ./scripts/validate-templates` checks every manifest. Add `-compose-dir <dir>` to also check a directory of plain compose files against the same rules a real deploy uses. Use this to triage a large catalog before converting entries to manifests. It prints one line per file with the first reason it was rejected. Add `-json` for machine-readable output and `-strict` to fail on any rejected file.

## Troubleshooting

### Compose file was rejected as invalid

**Likely cause:** The compose file uses keys that are not allowed (privileged, cap_add, host networking, bind mounts of docker.sock, etc.), or it has syntax errors.

**How to confirm:** Run `levelrail-cli compose preview -f compose.yaml`. The preview will show which keys are rejected and why.

**Fix:** Remove the rejected keys from the compose file. Common issues: privileged mode, cap_add, bind mounts to /var/run/docker.sock, and host networking are all refused for security. Update the file and try again.

### Service is not starting or is stuck waiting

**Likely cause:** A dependency is not healthy, or the service itself has a startup issue.

**How to confirm:** Run `levelrail-cli compose status <app>` and look for the per-service status. Check `depends_on` and `service_healthy` conditions in the compose file.

**Fix:** Check the service logs with `levelrail-cli compose logs <app> --service <service> -f`. If a dependency is not healthy, fix that service first (check its health check). If the service itself is failing, check logs for startup errors.

### Generated secret value is wrong or missing

**Likely cause:** The template is missing the setting definition, or the setting is not used in the compose file with the correct variable name.

**How to confirm:** Run `levelrail-cli templates get <id>` and check the `template.yaml` for the setting definition. Look for the `${KEY}` placeholder in the compose file.

**Fix:** Make sure the setting is defined in `template.yaml` and the variable name in the compose file exactly matches the `key` field. Generated values are stored per app and reused on redeploy, so environment changes to the setting do not affect existing apps.

### Still stuck

For help with compose files, run `levelrail-cli compose preview <name> -f compose.yaml` to see the full validation report. Check the app and service logs with `levelrail-cli compose logs <name> [--service <service>]`.

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/apps/{name}/compose[?profiles=a,b]` | `deploy` (plus `root` for bind mounts) |
| `POST` | `/api/v1/compose/preview[?profiles=a,b]` | `deploy` |
| `GET` | `/api/v1/compose` | `read` |
| `GET` | `/api/v1/apps/{name}/compose` | `read` |
| `GET` | `/api/v1/apps/{name}/compose/revisions[/{revision}]` | `read` |
| `POST` | `/api/v1/apps/{name}/compose/rollback` | `deploy` |
| `POST` | `/api/v1/apps/{name}/compose/redeploy` | `deploy` |
| `DELETE` | `/api/v1/apps/{name}/compose[?volumes=true]` | `write` (plus `write-sensitive` with volumes) |
| `POST` | `/api/v1/service-templates/{id}/install` | `deploy` |
