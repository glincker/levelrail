# app.yaml reference

The declarative spec Levelrail reads from your repo.

**Package**: `internal/spec` (see `internal/spec/spec.go`, `internal/spec/validate.go`, `internal/spec/schema/app.schema.json`).

**Filenames**: Levelrail looks for this file under these candidate names, in order:

- `app.yaml`
- `app.yml`
- `deploy.yaml`
- `deploy.yml`
- The current brand's own filename (see `internal/spec/discover.go`)

Generic names are checked first so a future product rename doesn't break a repo that already committed `app.yaml`.

## Full example

```yaml
version: 1
services:
  web:
    build:
      type: dockerfile        # dockerfile | compose | railpack | static
      path: ./Dockerfile
    domains:
      - app.example.com
    port: 3000
    host_port: 30001        # pin the host-side port; omit to let Docker assign one
    bind_address: private   # private (default, loopback only) | public | a literal IP
    health:
      readiness: { path: /healthz, interval: 5s, timeout: 2s }
      liveness:  { path: /healthz, interval: 30s, failures: 3 }
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
databases:
  main:
    engine: postgres          # postgres | redis | mysql | mongodb | mariadb | keydb | clickhouse | dragonfly
    version: "16"
    backup: { schedule: "0 3 * * *", retain: 7 }
    ephemeralInPreviews: true # opt in to a disposable per-pull-request instance, see below
```

This example matches what `internal/spec` actually parses and validates today. The test fixture is at `internal/spec/testdata/valid_full.yaml` (minus the plain-string env entry shown here; the fixture already has its own `labels` block).

Three additions beyond the project's planning doc, all implemented:

- `labels` block on a service
- `mysql` as a third supported database engine (alongside `postgres` and `redis`)
- Plain-string shorthand for a literal env value (shown here as `LOG_LEVEL`)

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
| `build` | `Build` | yes | none | How the service's image gets built. |
| `domains` | list of string | no | none | Public hostnames routed to this service. A domain can only be claimed by one service across the whole spec. |
| `port` | integer | conditional | none | 1 to 65535. Required unless `build.type` is `static`; must be omitted when `build.type` is `static`, since a static site has no running container to route to. |
| `host_port` | integer | no | auto-assigned | 1 to 65535. Pins the host-side port Docker binds `port` to. Omit to let Docker assign one. Must be omitted when `build.type` is `static`. |
| `bind_address` | string | no | `private` | `private` (loopback only), `public` (every interface, an explicit opt-in), or a literal IP (a specific host interface, or a WireGuard mesh peer address once that lands). Picks which network interface `port` (and `host_port`, if pinned) publishes to; see [Bind addresses and exposure](#bind-addresses-and-exposure) below. Must be omitted when `build.type` is `static`. |
| `health` | `Health` | no | none | Readiness and liveness probe configuration. |
| `resources` | `Resources` | no | none | Memory and CPU limits. |
| `env` | map of name to `EnvVar` | no | none | Environment variables. See the `EnvVar` shapes below. |
| `replicas` | integer | no | `1` | Minimum 1 if set. `Service.EffectiveReplicas()` returns this value or the default. |
| `strategy` | string | no | `blue-green` | One of `rolling`, `recreate`, `blue-green`. `Service.EffectiveStrategy()` returns this value or the default, chosen because blue-green is easier to get right than rolling with a single replica. |
| `labels` | map of string to string | no | none | Arbitrary operator-supplied Docker labels applied to the container at create time. See Validation below for the limits enforced on these. |
| `volumes` | list of `Volume` | no | none | Named Docker volumes and host-directory bind mounts this service's container mounts. |
| `hooks` | `Hooks` | no | none | Pre/post-deploy commands run inside the container. Not meaningful when `build.type` is `static` or `compose`. |
| `command` | list of string | no | none | Overrides the image's own default `CMD`. A plain argv list, never shell-interpreted. |

### `Build`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `type` | string | yes | none | One of `dockerfile`, `compose`, `railpack`, `static`. |
| `path` | string | conditional | none | Required when `type` is `compose`. Optional otherwise (for example, a non-default Dockerfile path). |
| `args` | map of string to string | no | none | Dockerfile build-time `ARG` values, passed through to BuildKit as `--build-arg` equivalents. Only meaningful when `type` is `dockerfile`. |

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
| `readiness` | `Probe` | no | none | Checked before cutting traffic to a new container. |
| `liveness` | `Probe` | no | none | Checked on a running container to detect a crashloop. |

### `Probe`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `path` | string | yes | none | HTTP path to check. |
| `interval` | string | no | none | Duration string matching `^[0-9]+(ms\|s\|m\|h)$`, for example `5s`. |
| `timeout` | string | no | none | Same duration pattern as `interval`. |
| `failures` | integer | no | none | Minimum 1 if set. |

### `Resources`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `memory` | string | no | none | Pattern `^[0-9]+(Mi\|Gi)$`, for example `512Mi`. |
| `cpu` | number | no | none | Must be greater than 0 if set. |
| `swapMemory` | string | no | none | Same pattern as `memory`. Docker's `MemorySwap`: the combined memory+swap ceiling, not swap on top of memory, so it must be at least `memory` and requires `memory` to also be set. |
| `cpuSet` | string | no | none | Docker's `cpuset-cpus` format, for example `0-3` or `0,2`. Pins the container to specific host CPUs. |

### `Hooks`

Shell commands the reconciler runs inside the service's own container via the Docker Engine API's exec facility (`sh -c <command>`), not by shelling out.

See `internal/reconcile/application/controller.go`'s doc comments for the full timing and failure-handling contract.

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
an object with these fields. `internal/spec/envvar.go` implements the
union via a custom `UnmarshalYAML`.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `from` | string | no | none | References another resource's computed value, for example `postgres.main.url`. |
| `secret` | boolean | no | `false` | The operator provides this value at deploy time through envelope-encrypted secret storage; it is never written to `app.yaml` or the git repo. |
| `required` | boolean | no | `false` | Only meaningful alongside `secret: true`: fail the deploy if no value has been provided, rather than starting the container with the variable unset. |
| `vault` | `VaultRef` | no | none | Resolves this value live from an external HashiCorp Vault instance instead of Levelrail's own envelope-encrypted storage. Mutually exclusive with `from` and `secret`. See [external secrets: HashiCorp Vault](deploying-apps.md#external-secrets-hashicorp-vault). |

The object form must set at least one of `from`, `secret`, or `vault`.
`vault` is mutually exclusive with both `from` and `secret`: a given env
var resolves its value from exactly one source.

### `VaultRef` (the value of `vault` on an `EnvVar`)

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

### `Database` (an entry under `databases`)

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `engine` | string | yes | none | One of `postgres`, `redis`, `mysql`, `mongodb`, `mariadb`, `keydb`, `clickhouse`, `dragonfly`. |
| `version` | string | no | none | For example `"16"`. |
| `backup` | `Backup` | no | none | Backup schedule. |
| `ephemeralInPreviews` | boolean | no | `false` | Provision a full, disposable database of its own for every pull-request preview, destroyed with the preview and with no restore path. Only meaningful once this app.yaml's git source has preview environments enabled; see [preview environments](roadmap.md) for the full lifecycle and its automatic env-var wiring. |

### `Backup`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `schedule` | string | yes | none | A cron expression, for example `"0 3 * * *"`. |
| `retain` | integer | no | none | Minimum 1 if set. |

## Bind addresses and exposure

Every published port (a service's `port`/`host_port`, and a managed database's public-access port; see [managing databases](managing-databases.md)) binds to `bind_address`, resolved by `internal/bindaddr.Resolve`:

| Value | Resolves to | Meaning |
| --- | --- | --- |
| `private` (default, or omitted) | `127.0.0.1` | Reachable only from this host. |
| `public` | `0.0.0.0` | Reachable from any network that can route to this host. Requires the literal string `public`; blank or malformed values never resolve here. |
| any other value | itself | Treated as a literal IP (a specific host interface or, once the WireGuard mesh lands, a mesh peer address). Must parse as a valid IP. |

### Default behavior

`private` is the default for anything created after this field shipped.

A service or database already publicly exposed before this field existed keeps that exposure across the upgrade. Both are backfilled to `public` via:

- `migrations/0098_service_bind_address.sql`
- `migrations/0099_database_public_bind_address.sql`

The new `private` default applies only on the next explicit redeploy or public-access change that leaves `bind_address` unset.

### Planned, not yet implemented

None found. Every field in this project's original app-spec design is
parsed and validated by `internal/spec` today.

## Validation

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
