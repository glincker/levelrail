# app.yaml reference

The declarative spec Levelrail reads from your repo. Package: `internal/spec`
(see `internal/spec/spec.go`, `internal/spec/validate.go`,
`internal/spec/schema/app.schema.json`).

Levelrail looks for this file under one of a few candidate names, in order:
`app.yaml`, `app.yml`, `deploy.yaml`, `deploy.yml`, plus whatever the current
brand's own filename resolves to (`internal/spec/discover.go`). The generic
names are checked first specifically so a future product rename doesn't
break a repo that already committed `app.yaml`.

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
    health:
      readiness: { path: /healthz, interval: 5s, timeout: 2s }
      liveness:  { path: /healthz, interval: 30s, failures: 3 }
    resources:
      memory: 512Mi
      cpu: 0.5
    env:
      DATABASE_URL: { from: postgres.main.url }
      API_KEY:      { secret: true, required: true }
      LOG_LEVEL: debug        # plain string shorthand for a literal value
    replicas: 2
    strategy: rolling         # rolling | recreate | blue-green
    labels:
      team: platform
      tier: frontend
databases:
  main:
    engine: postgres          # postgres | redis | mysql
    version: "16"
    backup: { schedule: "0 3 * * *", retain: 7 }
```

This matches what `internal/spec` actually parses and validates today
(`internal/spec/testdata/valid_full.yaml` is the equivalent fixture exercised
by the test suite). Two additions beyond the example in the project's own
planning doc, both real and implemented: the `labels` block on a service,
and `mysql` as a third supported database engine alongside `postgres` and
`redis`.

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
| `health` | `Health` | no | none | Readiness and liveness probe configuration. |
| `resources` | `Resources` | no | none | Memory and CPU limits. |
| `env` | map of name to `EnvVar` | no | none | Environment variables. See the `EnvVar` shapes below. |
| `replicas` | integer | no | `1` | Minimum 1 if set. `Service.EffectiveReplicas()` returns this value or the default. |
| `strategy` | string | no | `blue-green` | One of `rolling`, `recreate`, `blue-green`. `Service.EffectiveStrategy()` returns this value or the default, chosen because blue-green is easier to get right than rolling with a single replica. |
| `labels` | map of string to string | no | none | Arbitrary operator-supplied Docker labels applied to the container at create time. See Validation below for the limits enforced on these. |

### `Build`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `type` | string | yes | none | One of `dockerfile`, `compose`, `railpack`, `static`. |
| `path` | string | conditional | none | Required when `type` is `compose`. Optional otherwise (for example, a non-default Dockerfile path). |

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

### `EnvVar` (an entry under `env`)

Each value under `env` is either a plain string scalar (a literal value) or
an object with these fields. `internal/spec/envvar.go` implements the
union via a custom `UnmarshalYAML`.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `from` | string | no | none | References another resource's computed value, for example `postgres.main.url`. |
| `secret` | boolean | no | `false` | The operator provides this value at deploy time through envelope-encrypted secret storage; it is never written to `app.yaml` or the git repo. |
| `required` | boolean | no | `false` | Only meaningful alongside `secret: true`: fail the deploy if no value has been provided, rather than starting the container with the variable unset. |

The object form must set at least one of the three fields.

### `Database` (an entry under `databases`)

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `engine` | string | yes | none | One of `postgres`, `redis`, `mysql`. |
| `version` | string | no | none | For example `"16"`. |
| `backup` | `Backup` | no | none | Backup schedule. |

### `Backup`

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `schedule` | string | yes | none | A cron expression, for example `"0 3 * * *"`. |
| `retain` | integer | no | none | Minimum 1 if set. |

### Planned, not yet implemented

None found. Every field in this project's own planning example
(`CLAUDE.md` section 4.9) is parsed and validated by `internal/spec` today.

## Validation

`spec.Parse` (`internal/spec/spec.go`) runs two layers, and both must pass
before a caller ever sees a `Spec`:

1. **Structural shape**, checked against the embedded JSON Schema at
   `internal/spec/schema/app.schema.json` (compiled once and cached by
   `compiledAppSchema` in `internal/spec/schema.go`). This covers required
   fields, enums, string patterns (durations, memory sizes), numeric ranges,
   and rejecting unknown keys. YAML is decoded generically and round-tripped
   through JSON so the schema validator sees the same shapes it would from
   real JSON input.
2. **Semantic rules**, checked by `(*Spec).Validate` in
   `internal/spec/validate.go`, for anything the schema can't express because
   it depends on more than one field's own shape:
   - Service and database names must match `^[a-z][a-z0-9-]*$`.
   - A domain can only be claimed by one service in the whole spec.
   - `build.path` is required when `build.type` is `compose`.
   - `port` is required unless `build.type` is `static`, and forbidden when
     it is.
   - `strategy`, if set, must be one of the three known values (a second,
     redundant check behind the schema's own enum, since `Validate` is
     documented as safe to call on a hand-built `Spec` that never went
     through schema validation).
   - Service labels are checked by `ValidateLabels`
     (`internal/spec/labels.go`): at most 32 labels, keys up to 255
     characters, values up to 4096 characters, no empty keys, and no key
     starting with the reserved prefix `platform-reserved.` (kept open for
     the platform's own bookkeeping labels).

`spec.Parse` also runs `yamlUnmarshalStrict`, a YAML decode with
`KnownFields(true)`, as an independent second guard against the struct
tags and the JSON Schema drifting apart: an unknown key becomes a decode
error during development rather than a silently dropped field in
production.
