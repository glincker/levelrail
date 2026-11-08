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
| `ports` | The first container port is the one ingress routes to. A TCP host port is never published from compose. A UDP entry, `HOST:CONTAINER/udp` in short form or `protocol: udp` in long form, is published directly on the host (local node only), and TCP and UDP on the same number are separate ports. See [UDP published ports](/udp-ports). |
| `x-levelrail-domains` | Maps a service to a domain. HTTPS is issued automatically once DNS points at the server. |
| `volumes` | Named volumes become Docker volumes scoped to the app and service. An absolute host path is a bind mount and needs the `root` ability, for example `- /srv/cfg:/usr/share/nginx/html:ro` (read-only). Bind mounts work through `apps deploy-compose <name> --file`; `apps create --file` takes named volumes only. |
| `environment` | Literal values are stored as config. `${SERVICE_PASSWORD_X}` and the other generated kinds are created once, encrypted, and injected when the container is created. |
| `x-levelrail-strategy`, `x-levelrail-strategy-reason` | Per service: `recreate`, `rolling` or `blue-green` (the default). Use `recreate` for an app that locks its state volume (a database, for example), since the old container must stop first. Without it, a service keeps the start-first overlap unless it runs one replica with a writable volume or bind mount and also publishes a fixed host port or a stream, in which case the old container is stopped first automatically. That means brief downtime on each deploy; a failed readiness check still rolls back to the previous image. The reason is shown next to the strategy in the app's Deploy settings and in `apps get`. |
| `depends_on` | A dependent service is created only after its dependency's container runs (or already ran and exited 0). With `condition: service_healthy`, it also waits until the dependency passes its readiness check. With `condition: service_completed_successfully`, it waits until the dependency exits with code 0; a nonzero exit blocks it with a `DependencyFailed` reason. |
| `restart` | Only an explicit `restart: "no"` or `restart: on-failure` (also `on-failure:N`, N a non-negative integer that is accepted and ignored: there is no retry cap) makes a service run to completion, for a migration, init or seed container. Exit code 0 marks it `Completed`: it is not restarted, and its first deploy succeeds once it exits 0 (and fails on a nonzero exit). A long run is never stopped: past `APP_DEPLOY_ONESHOT_TIMEOUT` (default `30m`, or the service's own ready timeout) it reports `CompletionTimeout` and keeps running, and the deploy shows as failed with that reason until the service exits 0, when it flips back to done. A nonzero exit is restarted with crash backoff, as for any service. Any other value (for example `on-failure:abc` or `on-failure:-1`) gets a warning and is ignored. An unset `restart`, `always` and `unless-stopped` keep the default: the reconciler keeps the service running and restarts it after any exit. |
| `healthcheck` | An HTTP check (`curl` or `wget` against localhost) becomes an HTTP readiness probe. Any other command runs inside the container as an exec probe. |
| `x-levelrail-healthcheck` | Per service: a readiness probe the platform runs from outside the container against its published port, for images with no shell, `curl` or `wget` (built `FROM scratch` or distroless). It replaces `healthcheck:` here, so a file can keep `healthcheck:` for plain Compose. Fields: `kind` (`http` or `tcp`, required), `path` (http only, default `/`), `port` (must be the first TCP port, the default), `interval`, `timeout`, `retries`, `start_period`, `expect_status` (http only, for example `200-399`). Unset durations use `APP_PROBE_DEFAULT_INTERVAL` and `APP_PROBE_DEFAULT_TIMEOUT`. Redirects are never followed, the response body is not kept, and no credentials or environment values are sent. The deploy timeline notes the probe kind and its readiness result. |
| `profiles` | A service with `profiles:` only deploys when one of its profiles is enabled with `--profile-name` (or `?profiles=` on the API). |
| `deploy.replicas`, `deploy.resources.reservations.devices` | Replica count and GPU reservations. Other Swarm keys are rejected. |
| `mem_limit`, `cpus`, `deploy.resources.limits.memory`, `deploy.resources.limits.cpus` | Hard memory and CPU caps for the service (`512m`, `1g`, `0.5`). The shorthand wins when both are set. |
| `shm_size` | Size of `/dev/shm` (`1gb`, `256m`), for Chromium based apps that crash with Docker's 64 MB default. Changing it recreates the container. |
| `ulimits`, `init`, `extra_hosts`, `dns`, `sysctls`, `pids_limit`, `tmpfs`, `cap_drop`, `read_only`, `stop_grace_period` | Applied to the container, using the same allowlist as the `runtime:` block of an app spec. `stop_grace_period` sets the stop timeout; a value that does not parse as a duration is skipped with a warning. An invalid `cap_drop` or `read_only` fails the deploy, so a hardening setting is never dropped silently. Without `pids_limit`, the container gets the platform default (`APP_CONTAINER_PIDS_LIMIT`, 4096). Bounds come from the `APP_RUNTIME_*` variables. A change recreates the container. See [Runtime options](/runtime-options) for each key and its allowed values. |
| `networks` | Parsed and reported as a notice. Every service shares one app network. |

Values that are not finite, round to zero, are negative or exceed the cap are rejected at deploy time with the service and key named. Set `APP_COMPOSE_MAX_MEMORY_BYTES` and `APP_COMPOSE_MAX_CPUS` on the control plane to lower the cap; unset keeps only the built-in overflow guard. `shm_size` is capped at 2 GiB by default; set `APP_COMPOSE_MAX_SHM_BYTES` (bytes) to raise or lower it. A service with `shm_size` and no memory limit counts its `/dev/shm` size against the project memory quota, since that memory comes from the host.

### Keys that are ignored

A key outside the table above does not stop the deploy, but it is never dropped silently. Every ignored key is reported as a warning that names the service, the key and what to do instead. `levelrail-cli apps deploy-compose` prints each one with the file path, `compose preview` and `apps validate` list them, and the deploy API returns them in `notices` (level `warning`).

The warning has one fixed shape, with `top-level` in place of the service name for a top-level key:

```text
service "web": container_name: is not supported and was ignored; the custom name is not applied; containers are named <app>-<service> and other services reach this one by its service key, so use that as the hostname
```

A key with no specific advice gets `remove it, or bake the setting into the image; it has no effect on how this service runs`.

### env_file and extends

`env_file` (a path, a list, or `{path, required}` entries) is read only for git deploys with `build.type: compose`. Paths are relative to the compose file's directory and must stay inside the repository: absolute paths, `..` escapes and symlinks leaving the checkout are rejected, as is a file larger than `APP_COMPOSE_MAX_ENV_FILE_BYTES` (default 256 KiB; the variable can only lower it). A missing file is an error naming the service and path, unless the entry has `required: false`. Files apply in order, later files win, and `environment:` overrides them all. Lines are `KEY=VALUE` with optional `export`, `#` comments and single or double quotes. Values are used as written, with no `${VAR}` expansion, the same as `environment:` in a git deploy. Error messages carry line numbers, never values.

`extends: {service: name}` pointing at a service in the same file is merged before the file is read. `command` and `entrypoint` are replaced, `environment` and `labels` merge by key, `ports`, `expose` and `env_file` are concatenated, `volumes` merge by mount path, other maps merge deeply, and `depends_on`, `links` and `volumes_from` are not inherited. A missing parent or a cycle is an error. `extends` with `file:` is still ignored with a warning.

### Supported subset

These service keys are read. Everything else is ignored with a warning, or rejected (see below). The list comes from the parser in `internal/compose`.

| Group | Keys |
| --- | --- |
| Run | `image`, `command`, `entrypoint`, `user`, `working_dir`, `environment`, `env_file` (git deploys), `extends` (same file), `restart`, `pull_policy`, `profiles`, `labels` |
| Network and storage | `ports`, `volumes`, `networks` (reported only) |
| Readiness and order | `healthcheck` (`test`, `interval`, `timeout`, `retries`, `start_period`), `depends_on` |
| Resources | `mem_limit`, `cpus`, `shm_size`, `deploy` (`replicas`, `resources.limits`, `resources.reservations.devices`) |
| Runtime | `ulimits`, `init`, `extra_hosts`, `dns`, `sysctls`, `pids_limit`, `tmpfs`, `cap_drop`, `read_only`, `stop_grace_period`; see [Runtime options](/runtime-options) |
| Levelrail extensions | `x-levelrail-domains`, `x-levelrail-strategy`, `x-levelrail-strategy-reason`, `x-levelrail-healthcheck`, `x-levelrail-published-ports` |
| Top level | `services`, `version`, `networks`, `volumes` (names only) |

`build`, `secrets`, `configs` and the host access keys listed under "What is rejected" are parsed only so they can be refused or redirected with a clear message. They are not supported. `SERVICE_FQDN_*`, `SERVICE_URL_*` and `SERVICE_PASSWORD_*` style variables from imported templates are rewritten into literal values or generated secrets before the file is parsed.

The ignored keys people paste most, and the fix for each:

| Key | Fix |
| --- | --- |
| `env_file` | Read for git deploys (`build.type: compose`), then overridden by `environment:`. In a pasted file it is not read: move the values into `environment:` or the app's env vars. |
| `extends` | Same-file `extends: {service: name}` is merged. Only `extends` with `file:` is ignored: inline those keys. |
| `container_name` | Not applied. Containers are named `<app>-<service>`; other services reach this one by its service key. |

More ignored keys and their alternatives:

| Ignored key | Instead |
| --- | --- |
| `extends` with `file:` | Inline the keys from the other file. |
| `env_file` in a pasted file | Move the values into `environment:` or the app's env vars. |
| `container_name`, `hostname`, `links` | Not applied or needed: containers are named `<app>-<service>` and services reach each other by name. |
| `expose` | Add `ports:` for the port ingress should route to. |
| `deploy.resources.reservations` CPU and memory | Only limits are applied. |
| `tty`, `stdin_open`, `logging` | Remove them, they have no effect. |
| Top-level `volumes:` options, `name:` | Named volumes are always created for the app. |
| `healthcheck.disable`, `healthcheck.start_interval`, `depends_on.restart`, `depends_on.required`, long-form `ports` `host_ip`, long-form `volumes` `bind` or `tmpfs` | Remove them. |

`x-` extension fields are ignored by design, as the Compose spec allows. Anchors and merge keys (`<<: *common`) are resolved before keys are checked.

### What is rejected

These keys widen a container's access to the host, so a compose file that sets any of them is rejected with a clear error. The rule name in brackets matches the Docker socket guard ([Docker access](/docker-access)):

- `privileged: true` [privileged]
- `network_mode: host` [network_host], and `network_mode: service:...` or `container:...`
- `pid`, `ipc`, `uts`, `userns_mode` or `cgroup` set to `host`
- `cap_add` (so no `NET_ADMIN` yet, see [UDP published ports](/udp-ports#not-yet)), `devices`, `volumes_from`, and `security_opt` (except `no-new-privileges`)
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

## Pull request previews

When a compose app has pull request previews on, every service in the preview gets `PR_NUMBER` and `PREVIEW_PR_NUMBER` (the pull request number), `PREVIEW_BRANCH`, and `PREVIEW_URL` (only on the service that carries the preview domain, and only when a domain was assigned). Use them in your own env or entrypoint, for example to name a preview database. A `PR_NUMBER` your app already defines is kept.


## Env vars per service

Every compose service is a member app named `<app>-<service>`, and its env belongs to that member only. A variable reaches exactly the services that declare it, and nothing is copied to siblings. To set a value for one service, address its member app:

```bash
levelrail-cli apps env set blog-db POSTGRES_USER=blog
levelrail-cli apps env export blog-db
```

Effective env of one service, lowest to highest precedence: the `environment:` block in the compose file, then env set on the member app, then branch or pull request overrides on that member for previews. Generated secrets are encrypted per member and masked when listed. To give several services the same value, set it on each member.

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

## Backing up a database service

Postgres, MySQL and MariaDB services in a compose app (detected by image name: `postgres`, `postgis`, `mysql`, `mariadb`) can be dumped on a cron schedule. The dump runs inside the service container with the engine's own tool (`pg_dump`, `mysqldump`, `mariadb-dump`), using the credentials already in the container's environment (`POSTGRES_USER`, `POSTGRES_DB`, `MYSQL_ROOT_PASSWORD`, `MARIADB_ROOT_PASSWORD`). The MySQL family password goes through `MYSQL_PWD`, never a command line. The dump then uses the same backup target, encryption, upload and retention as managed database backups. Dumps run on the control plane node, so the service must run there.

Schedules and run history for compose services are stored apart from managed database history. Each run is recorded with its status, size, object key and error.

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/apps/{name}/compose-db-services` | `read` (detected database services and their schedules) |
| `PUT` | `/api/v1/apps/{name}/compose-db-services/{service}/backup-schedule` | `write-sensitive` (`target_id`, `schedule`, `retain`, `retain_days`, `enabled`) |
| `DELETE` | `/api/v1/apps/{name}/compose-db-services/{service}/backup-schedule` | `write-sensitive` |
| `GET` | `/api/v1/apps/{name}/compose-db-services/{service}/backups` | `read` (run history) |
| `POST` | `/api/v1/apps/{name}/compose-db-services/{service}/backups` | `write-sensitive` (run now, `target_id` optional when a schedule exists) |

Restore is not automated for compose services yet. Download the dump object from the backup target (decrypt it first if the target encrypts dumps), then load it into the running service by hand:

```sh
# Postgres
docker exec -i <container> sh -c 'psql -U "${POSTGRES_USER:-postgres}" "${POSTGRES_DB:-${POSTGRES_USER:-postgres}}"' < dump.sql
# MySQL or MariaDB
docker exec -i <container> sh -c 'MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}" mysql -uroot' < dump.sql
```

Take a fresh dump first, since loading over live data can overwrite it. Dashboard and CLI controls for these schedules are not part of this change.
