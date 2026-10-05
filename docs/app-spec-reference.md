---
description: Complete reference for app.yaml - the declarative app spec for Levelrail
---

# app.yaml reference

`app.yaml` is the declarative spec Levelrail reads from your repo: services, their build and runtime settings, and the managed databases they use. Every field below is parsed and validated against the JSON Schema embedded in the control plane (`internal/spec/schema/app.schema.json`), and an unknown key is an error rather than being ignored.

Levelrail looks for the file under these names, in order: `app.yaml`, `app.yml`, `deploy.yaml`, `deploy.yml`, then the product's own name with a `.yaml` or `.yml` extension. The generic names come first so a repo that already committed `app.yaml` keeps working.

<InlineToc default-open />

## Full example

```yaml
version: 1
services:
  web:
    build:
      type: dockerfile        # dockerfile | compose | railpack | static | image
      path: ./Dockerfile
      baseDirectory: apps/web # build context subdirectory, for monorepos
    domains:
      - app.example.com
    port: 3000
    host_port: 30001        # pin the host-side port; omit to let Docker assign one
    bind_address: private   # private (default, loopback only) | public | a literal IP
    health:
      readiness: { path: /healthz, interval: 5s, timeout: 2s }
      liveness:  { path: /healthz, interval: 30s, failures: 3 }
      readyTimeout: 90s     # how long a fresh deploy waits for readiness before failing; default 60s
    resources:
      memory: 512Mi
      cpu: 0.5
    env:
      DATABASE_URL: { from: postgres.main.url }
      API_KEY:      { secret: true, required: true }
      STRIPE_KEY:   { vault: { path: myapp/config, key: stripe_key } }
      LOG_LEVEL: debug        # plain string shorthand for a literal value
    replicas: 2
    strategy: rolling         # rolling | recreate | blue-green
    labels:
      team: platform
      tier: frontend
    volumes:
      - name: data              # named Docker volume
        path: /var/lib/data
      - hostPath: /srv/web/uploads  # bind mount of a real host directory
        path: /uploads
        readOnly: true
    hooks:
      preDeploy: rails db:migrate
      postDeploy: curl -f https://hooks.example.com/deployed
    command: ["node", "server.js", "--port", "3000"]  # overrides the image's own CMD
    egress:
      mode: allowlist       # outbound traffic limited to the hosts below
      allow:
        - { host: api.stripe.com, port: 443 }
    dependsOn: [worker]     # start order across sibling services
databases:
  main:
    engine: postgres          # postgres | redis | mysql | mongodb | mariadb | keydb | clickhouse | dragonfly
    version: "16"
    backup: { schedule: "0 3 * * *", retain: 7 }
    ephemeralInPreviews: true # opt in to a disposable per-pull-request instance, see below
```

A prebuilt image needs no build block: `image: ghcr.io/your-org/web:1.0` on the service is shorthand for `build: { type: image, image: ... }`.

## Field reference

### Top level (`Spec`)

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `version` | integer | yes | none | Must be exactly `1` (`schema: const: 1`). |
| `services` | map of name to `Service` | yes | none | At least one service required. Keys must match `^[a-z][a-z0-9-]*$` (lowercase alphanumeric and hyphens, starting with a letter). |
| `databases` | map of name to `Database` | no | none | Same key pattern as `services`. |

### `Service` (an entry under `services`)

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `build` | `Build` | one of `build`/`image` | none | How the service's image gets built. |
| `image` | string | one of `build`/`image` | none | Shorthand for `build: {type: image, image: ...}`: deploy a prebuilt image as-is. Works with `replicas`, `strategy` and every other service field. Setting both is an error unless `build.type` is `image` with the same reference. |
| `domains` | list of string | no | none | Public hostnames routed to this service. A domain can only be claimed by one service across the whole spec. |
| `port` | integer | conditional | none | 1 to 65535. Required unless `build.type` is `static`; must be omitted when `build.type` is `static`, since a static site has no running container to route to. |
| `host_port` | integer | no | auto-assigned | 1 to 65535. Pins the host-side port Docker binds `port` to. Omit to let Docker assign one. Must be omitted when `build.type` is `static`. |
| `bind_address` | string | no | `private` | `private` (loopback only), `public` (every interface, an explicit opt-in), or a literal IP (a specific host interface or a WireGuard mesh peer address). Picks which network interface `port` (and `host_port`, if pinned) publishes to; see [Bind addresses and exposure](#bind-addresses-and-exposure) below. Must be omitted when `build.type` is `static`. |
| `health` | `Health` | no | none | Readiness and liveness probe configuration. |
| `resources` | `Resources` | no | none | Memory and CPU limits. |
| `env` | map of name to `EnvVar` | no | none | Environment variables. See the `EnvVar` shapes below. |
| `replicas` | integer | no | `1` | Minimum 1 if set. |
| `strategy` | string | no | `blue-green` | One of `rolling`, `recreate`, `blue-green`. |
| `labels` | map of string to string | no | none | Arbitrary operator-supplied Docker labels applied to the container at create time. See Validation below for the limits enforced on these. |
| `volumes` | list of `Volume` | no | none | Named Docker volumes and host-directory bind mounts this service's container mounts. |
| `hooks` | `Hooks` | no | none | Pre/post-deploy commands run inside the container. Not meaningful when `build.type` is `static` or `compose`. |
| `command` | list of string | no | none | Overrides the image's own default `CMD`. A plain argv list, never shell-interpreted. |
| `egress` | `Egress` | no | open egress | Limits outbound traffic to an allowlist of host and port pairs. See [`Egress`](#egress). |
| `loadbalancer` | `LoadBalancer` | no | none | Balances traffic across the service's replicas. See [Load balancing](load-balancing.md). |
| `dependsOn` | list of string | no | none | Sibling `services:` keys this service waits on: the reconciler does not create this service's container until every named dependency has at least one running container. Start order only, real Docker Compose's own default `depends_on:` semantic (`service_started`), not a wait for the dependency's own readiness or health check. Each entry must name a real sibling service with a single running container (not `static` or `compose`), and a cycle between services fails validation. |

### `Build`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `type` | string | yes | none | One of `dockerfile`, `compose`, `railpack`, `static`, `image`. |
| `path` | string | conditional | none | Required when `type` is `compose`. Optional for `dockerfile`, `railpack` and `static` (for example, a non-default Dockerfile path). Rejected for `image`. |
| `baseDirectory` | string | no | repo root | Subdirectory used as the build context, for a monorepo (`apps/web`). Must be relative and stay inside the repo. Rejected for `image` and `compose` (a compose file's own `context:` already scopes each service). |
| `image` | string | conditional | none | Full registry reference such as `ghcr.io/org/app:v1.2.3`. Required when `type` is `image`, rejected otherwise. Nothing is built; the image is deployed as is. |
| `registryCredential` | string | no | none | Name of a registry credential saved in Levelrail, used to pull `image` from a private registry. Empty means an unauthenticated pull. |
| `args` | map of string to string | no | none | Dockerfile build-time `ARG` values, passed through to BuildKit as `--build-arg` equivalents. Only meaningful when `type` is `dockerfile`. |

### `Egress`

Opt-in outbound allowlist for one service. Without it the service's outbound traffic is unrestricted. Not valid with `build.type` of `static` or `compose`.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `mode` | string | yes | Only `allowlist` is supported. |
| `allow` | list of `{host, port}` | yes, at least one entry | `host` is a hostname or IPv4 address (re-resolved periodically, not pinned at deploy time). `port` is 1 to 65535. |

### `Volume` (an entry under `volumes`)

Exactly one of `name` or `hostPath` must be set:

- `name`: declares a named Docker volume (scoped to this service)
- `hostPath`: a bind mount of a real directory on the node where the service runs

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string | conditional | none | A named Docker volume, scoped to this service (two services can each declare a volume named `data` without colliding). Must match `^[a-z][a-z0-9-]*$`. |
| `hostPath` | string | conditional | none | An absolute path on the host to bind-mount. Rejected if relative or under a protected system path (see Validation below). |
| `path` | string | yes | none | The container-side mount path. |
| `readOnly` | boolean | no | `false` | Mounts read-only inside the container. Only meaningful with `hostPath`; rejected on a named volume. |

**Constraint**: Two volumes on the same service can never share a `path`.

### `Health`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `readiness` | `Probe` | no | none | Checked before cutting traffic to a new container. See [Health checks](#health-checks). |
| `liveness` | `Probe` | no | none | Checked on a running container to detect a crashloop. |
| `readyTimeout` | string | no | `60s` | Duration string matching `^[0-9]+(ms\|s\|m\|h)$`. How long a fresh deploy waits for `readiness` to pass before the deploy is marked `ReadinessFailed`. Raise this for a service with a genuinely slow cold start (JVM warm-up, a large migration, a slow external connection) instead of it being falsely flagged as failed; the platform still self-heals on the next resync once the container is actually healthy, but this avoids the false signal in the meantime. |

### `Probe`

A probe is either an HTTP(S) request (set `path`) or a command run inside the container (set `exec`), never both.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `path` | string | one of `path`/`exec` | none | HTTP path to request, starting with `/`. |
| `scheme` | string | no | `http` | `http` or `https`. |
| `host` | string | no | none | `Host` header to send, and the TLS server name for certificate verification. A bare hostname or `host:port`. |
| `tls_skip_verify` | boolean | no | `false` | Accept any certificate. Only valid with `scheme: https`; meant for the self-signed certificates most images generate for themselves. Verification stays on unless this is set. |
| `follow_redirects` | boolean | no | follow | `false` judges a 3xx response itself against `expected_status` instead of following it. Left unset, redirects are followed (up to `APP_PROBE_MAX_REDIRECTS`, default 10), which is how probes behaved before this field existed. |
| `expected_status` | string, integer, or list | no | `200-299` | Accepted status codes: a code (`204`), a range (`200-399`), a comma-separated string (`"200,301-302"`), or a list (`[200, "301-302"]`). |
| `exec` | string or list | one of `path`/`exec` | none | A string runs through `/bin/sh -c`; a list runs as argv (for images with no shell). Exit code 0 means healthy. Runs through the Docker Engine API, bounded by `timeout`; its output (capped at `APP_PROBE_EXEC_OUTPUT_BYTES`, default 512) shows up in the failure reason. A command that hangs past `timeout` is reported as failed, but Docker has no API to kill an exec'd process, so it keeps running inside the container. |
| `interval` | string | no | `2s` (`APP_PROBE_DEFAULT_INTERVAL`) | Duration string matching `^[0-9]+(ms\|s\|m\|h)$`, for example `5s`. |
| `timeout` | string | no | `2s` (`APP_PROBE_DEFAULT_TIMEOUT`) | Same duration pattern as `interval`. |
| `failures` | integer | no | none | Minimum 1 if set. |

### Health checks

Readiness gates a deploy's cutover: the new container has to pass it before the old one is retired. Liveness runs on every reconcile pass against a running container and restarts it after `failures` consecutive failures. An HTTP probe needs a `port`; an exec probe does not, so a worker with nothing listening can still have one.

```yaml
health:
  readiness:
    path: /api/health
    scheme: https
    tls_skip_verify: true      # self-signed certificate inside the container
    follow_redirects: false
    expected_status: 200-399   # a login redirect counts as up
  liveness:
    exec: pg_isready -U app    # or [pg_isready, -U, app]
    timeout: 5s
    failures: 3
```

Every probe result lands in the app's `Ready` condition (reason `ReadinessFailed`, `RunningNotReady`, `LivenessDegraded`, and so on), and the message says exactly what failed, for example:

- `GET https://127.0.0.1:32768/health returned 302 to /login; set follow_redirects or expected_status (currently 200-299)`
- `GET https://127.0.0.1:32768/ : TLS verification failed: x509: certificate signed by unknown authority; set tls_skip_verify for a self-signed certificate`
- `exec "redis-cli -p 6390 ping" exited 1: Could not connect to Redis at 127.0.0.1:6390: Connection refused`
- `GET http://127.0.0.1:32768/healthz timed out after 2s`

The same settings are editable in the dashboard (app, then Health), with `levelrail-cli apps health set`, and through `PUT /api/v1/apps/{name}/health`.

A Compose file's `healthcheck:` is translated into a readiness probe: a `curl`/`wget` command becomes an HTTP(S) probe that keeps the tool's own semantics (`curl -L` follows redirects, `curl -f` accepts any status below 400, `-k`/`--no-check-certificate` skip TLS verification), `start_period` widens the readiness budget, and any other command (`pg_isready`, `redis-cli ping`, `mysqladmin ping`) becomes an exec probe. A bare `/dev/tcp` connect check is not translated.

### `Resources`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `memory` | string | no | none | Pattern `^[0-9]+(Mi\|Gi)$`, for example `512Mi`. |
| `cpu` | number | no | none | Must be greater than 0 if set. |
| `swapMemory` | string | no | none | Same pattern as `memory`. Docker's `MemorySwap`: the combined memory+swap ceiling, not swap on top of memory, so it must be at least `memory` and requires `memory` to also be set. |
| `cpuSet` | string | no | none | Docker's `cpuset-cpus` format, for example `0-3` or `0,2`. Pins the container to specific host CPUs. |
| `gpu` | `all`, integer, or object | no | none | Requests NVIDIA GPUs. `all`, a count such as `2`, or `{count: 2}` / `{devices: ["0", "GPU-uuid"]}` (device indexes or UUIDs win over count). The app is refused on a node without a GPU and the nvidia container runtime, and moving it to such a node is rejected. See [AI models](ai-models.md#gpu-nodes). |

### `LoadBalancer`

All fields are optional. Durations are Go duration strings such as `500ms`, `5s` or `2m`. See [Load balancing](load-balancing.md) for behavior.

| Field | Type | Description |
| --- | --- | --- |
| `algorithm` | string | `round_robin` (default), `least_conn`, `ip_hash`, `uri_hash`, `cookie` or `weighted`. |
| `cookie_name` | string | Sticky cookie name. Only with `algorithm: cookie`, default `lb`. |
| `weights` | list of integer | 1 to 100 per replica, indexed by replica number. Only with `algorithm: weighted`. |
| `active_health` | object | `path` (required, starts with `/`), `interval`, `timeout` (shorter than `interval`), `passes`, `fails`, `expect_status`. |
| `passive_health` | object | `max_fails`, `fail_duration`. |
| `retries` | object | `count` (0 to 10), `try_duration`, `try_interval`. |
| `slow_start` | duration | Weight ramp for a new replica. Weighted only. |
| `drain_timeout` | duration | How long open streams survive a cutover. |
| `request_timeout` | duration | Upstream response header timeout. |
| `rate_limit` | object | `rps` (required), `burst`. Per client address. |
| `upstream_tls` | object | `server_name`, `insecure_skip_verify`. Speak HTTPS to replicas. |

### `Hooks`

Shell commands the reconciler runs inside the service's own container via the Docker Engine API's exec facility (`sh -c <command>`), not by shelling out.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `preDeploy` | string | no | none | Runs once per deploy, before the new container's readiness probe and before any old container is removed. A nonzero exit blocks the deploy: the new container is rolled back and whatever was previously running keeps serving. |
| `postDeploy` | string | no | none | Runs once per deploy, after the full replica set has cut over and any stale container has been removed. A nonzero exit is reported (`PostDeployHookFailed` reconcile condition) but never rolls back an already-successful cutover. |

**Rules**

- At least one of `preDeploy`/`postDeploy` must be set; an empty `hooks: {}` block is rejected.
- With more than one replica, each hook runs exactly once per deploy (against the first replica), not once per replica. Running a database migration N times per deploy would be wrong even though every replica gets a freshly built container from the same image.
- The most recent outcome of each hook (exit code, captured output) is queryable via `GET /api/v1/apps/{name}/hook-runs`.

### `EnvVar` (an entry under `env`)

Each value under `env` is either a plain string scalar (a literal value) or
an object with these fields. 

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `from` | string | no | none | References another resource's computed value, for example `postgres.main.url`. |
| `secret` | boolean | no | `false` | The operator provides this value at deploy time through envelope-encrypted secret storage; it is never written to `app.yaml` or the git repo. |
| `required` | boolean | no | `false` | Only meaningful alongside `secret: true`: fail the deploy if no value has been provided, rather than starting the container with the variable unset. |
| `vault` | `VaultRef` | no | none | Resolves this value live from an external HashiCorp Vault instance instead of Levelrail's own envelope-encrypted storage. Mutually exclusive with `from` and `secret`. See [external secrets: HashiCorp Vault](deploying-apps.md#external-secrets-from-hashicorp-vault). |

The object form must set at least one of `from`, `secret`, or `vault`.
`vault` is mutually exclusive with both `from` and `secret`: a given env
var resolves its value from exactly one source.

### `VaultRef` (external secrets integration)

::: details VaultRef field reference
| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `path` | string | yes | none | The secret's path in Vault's KV v2 engine, for example `myapp/config`. Resolved against the mount path configured under Settings > Vault (default `secret`). |
| `key` | string | yes | none | The field name inside that secret's data, for example `api_key`. |

```yaml
env:
  API_KEY: { vault: { path: myapp/config, key: api_key } }
```

The value is read fresh from Vault immediately before the container is created and is never persisted by Levelrail.

If Vault is unreachable, not configured, or the secret/field doesn't exist, the deploy fails loudly rather than starting the container with the variable empty.
:::

### `Database` (an entry under `databases`)

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `engine` | string | yes | none | One of `postgres`, `redis`, `mysql`, `mongodb`, `mariadb`, `keydb`, `clickhouse`, `dragonfly`. |
| `version` | string | no | none | For example `"16"`. |
| `backup` | `Backup` | no | none | Backup schedule. |
| `ephemeralInPreviews` | boolean | no | `false` | Provision a full, disposable database of its own for every pull-request preview, destroyed with the preview and with no restore path. Only meaningful once the app's git source has preview environments enabled; see [Git integrations](git-integrations.md) and [Deploy previews](deploy-previews.md). |
| `isolatedInPreviews` | boolean | no | `false` | Give each preview its own credential on the existing database (a Postgres role or a Redis ACL user scoped to its own key prefix) instead of a new instance. Postgres and Redis only. Ignored when `ephemeralInPreviews` is also set. |

### `Backup`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `schedule` | string | yes | none | A cron expression, for example `"0 3 * * *"`. |
| `retain` | integer | no | none | Minimum 1 if set. |

## Bind addresses and exposure

Every published port (a service's `port`/`host_port`, and a managed database's public-access port; see [managing databases](managing-databases.md)) binds to `bind_address`, resolved by `internal/bindaddr.Resolve`:

| Value | Resolves to | Meaning |
| --- | --- | --- |
| `private` (default, or omitted) | `127.0.0.1` | Reachable only from this host. On a remote node with a healthy WireGuard mesh, the main port binds to that node's mesh IP instead (never public), so the control plane's ingress can reach it; see [multi-node](multi-node.md#routing-to-apps-on-remote-nodes). |
| `public` | `0.0.0.0` | Reachable from any network that can route to this host. Requires the literal string `public`; blank or malformed values never resolve here. |
| any other value | itself | Treated as a literal IP (a specific host interface or, once the WireGuard mesh lands, a mesh peer address). Must parse as a valid IP. |

### Default behavior

`private` is the default for anything created after this field shipped. A service or database that was already publicly exposed before the field existed was backfilled to `public` and keeps that exposure. The `private` default applies only when a redeploy or public-access change leaves `bind_address` unset on a newly created resource.

## Validation

::: details Validation rules and implementation
`spec.Parse` (`internal/spec/spec.go`) runs two layers. Both must pass before a caller sees a `Spec`.

### Layer 1: Structural shape

Checked against the embedded JSON Schema at `internal/spec/schema/app.schema.json` (compiled and cached by `compiledAppSchema` in `internal/spec/schema.go`).

Coverage:

- Required fields
- Enums
- String patterns (durations, memory sizes)
- Numeric ranges
- Rejection of unknown keys

YAML is decoded generically and round-tripped through JSON so the schema validator sees the same shapes as real JSON input.

### Layer 2: Semantic rules

Checked by `(*Spec).Validate` in `internal/spec/validate.go` for anything the schema can't express because it depends on multiple fields.

**Names**

- Service and database names must match `^[a-z][a-z0-9-]*$`.
- A domain can only be claimed by one service in the whole spec.

**Build configuration**

- `build.path` is required when `build.type` is `compose`.
- `build.image` is required when `build.type` is `image` and rejected for any other type. `build.path` and `build.baseDirectory` are rejected for `image`, and `build.baseDirectory` is rejected for `compose`.
- `build.baseDirectory` must be relative and must not escape the repository root.
- `build.args` is only meaningful when `build.type` is `dockerfile`. Set with any other build type, it is rejected rather than silently ignored.

**Port configuration**

- `port` is required unless `build.type` is `static`.
- `port` is forbidden when `build.type` is `static`.

**Strategy**

- `strategy`, if set, must be one of the three known values (a redundant check behind the schema's enum; `Validate` is documented as safe to call on a hand-built `Spec` that never went through schema validation).

**Service labels**

Checked by `ValidateLabels` (`internal/spec/labels.go`):

- At most 32 labels per service
- Keys up to 255 characters
- Values up to 4096 characters
- No empty keys
- No key starting with the reserved prefix `platform-reserved.` (kept open for the platform's own bookkeeping labels)

**Resource limits**

- `resources.swapMemory` requires `resources.memory` to also be set.
- `resources.swapMemory` must be at least `resources.memory` (checked during deploy translation in `internal/deploy`, since both values need byte conversion). Docker's `MemorySwap` is the combined memory+swap ceiling, not swap on top of memory.

**Egress**

- `egress` is rejected when `build.type` is `static` or `compose`. `mode` must be `allowlist` with at least one `allow` entry, each a hostname or IPv4 address with a valid port.

**Dependencies**

- Each `dependsOn` entry must name another service in the file, not itself, and not a `static` or `compose` service. A cycle is rejected.

**Hooks**

- `hooks` is rejected when `build.type` is `static` (no container to run a command in) or `compose` (a wrapper that expands into N real services; one `hooks` block cannot unambiguously target any of them).
- An empty `hooks: {}` block is rejected at the schema layer (the schema's own `minProperties: 1`).

**Volumes**

- Each volume's `name` (if a named volume) must match `^[a-z][a-z0-9-]*$`.
- No two volumes on the same service may share a name or the same `path`.
- Each volume's `hostPath` (if a bind mount) must be an absolute path.
- `hostPath` is rejected under protected system paths: `/`, `/etc`, `/root`, `/boot`, `/sys`, `/proc`, `/var/lib/docker`, `/var/run/docker.sock`, or `/var/run`. `/var/run/docker.sock` is excluded by design (it's a full container-escape-to-host-root vector; this feature doesn't grant that capability).
- `readOnly: true` on a volume with `name` set (not `hostPath`) is rejected (no meaning for a named Docker volume).

### Additional guard: strict YAML parsing

`spec.Parse` also runs `yamlUnmarshalStrict` (YAML decode with `KnownFields(true)`) as an independent guard against struct tags and JSON Schema drifting apart. An unknown key becomes a decode error during development rather than a silently dropped field in production.
:::

## Next steps

<CardGroup :cols="2">
<Card title="Getting started" href="/getting-started">

Deploy your first app with an `app.yaml`.

</Card>
<Card title="Deploying apps" href="/deploying-apps">

Secrets, environment setup, rollouts and rollbacks.

</Card>
<Card title="Managing databases" href="/managing-databases">

Database configuration and backups.

</Card>
<Card title="Git integrations" href="/git-integrations">

Branch, push versus release trigger, and webhooks. None of that lives in `app.yaml`.

</Card>
</CardGroup>
