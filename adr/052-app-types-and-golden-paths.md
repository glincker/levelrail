# ADR 052: App types and golden paths

Status: Proposed

Date: 2026-10-29

## Context

The question: can a platform admin define a governed "type" of app
(for example "internal HTTP API" or "nightly batch job") that developers
pick, with typed parameters, per-environment settings kept apart from
per-app settings, and an allowlist of build methods and add-ons? Today
the answer is no. Verified on `release/v0.8.11` by reading the code:

1. **The service template catalog is a one-shot starting point.**
   `catalog.Template` (`internal/catalog/catalog.go:14`) carries an id,
   name, category, a Compose body, a RAM advisory, a GPU flag and an
   optional `Manifest` for entries loaded from
   `internal/catalog/manifests/` (37 files). The Go files hold about 379
   `ID:` entries, consistent with the roughly 360 entries ADR 015 talks
   about. A template is served by `GET /api/v1/service-templates` and
   copied into an app; nothing links the app back to the template
   afterwards, and nothing restricts which templates a team may use.
2. **Framework presets already have typed parameters, in code.**
   `internal/projecttpl/projecttpl.go` defines `Param` (`:27`) with
   kinds `string`, `int`, `enum` and `cron`, `Default`, `Required`,
   `Options` and `Placeholder`. Five templates live in a Go slice
   (`:88`): `web-service`, `static-site`, `worker`, `cron-job`,
   `build-test-deploy`. `resolve` (`:136`) and `check` (`:170`) return a
   `ValidationError` listing every bad field with a human message, which
   is the validation shape this ADR wants. But the list is compiled in,
   rendering produces files once (`Render`, `:118`), and the build
   method enum is hardcoded to `railpack` and `dockerfile` (`:83`).
3. **Deploy policies are the existing enforcement point.**
   `internal/policy/policy.go` has scopes `org`, `project`,
   `environment` (`:13` to `:15`), modes `off`, `warn`, `enforce`
   (`:25` to `:27`), and `Merge` where the most specific scope wins per
   rule (`:155`). `Subject` (`:77`) only knows image, memory, CPU,
   readiness, privileged, host mounts, labels and replicas. The gate is
   `policyGate` in `internal/api/deploy_policy.go:91`: 422 with code
   `policy_violation` and a `violations` list, override needs the root
   ability plus a reason and records a `policy.override` app event.
   The same evaluation runs from scheduled deploys
   (`internal/api/scheduled_deploys.go:224`), pull request comment
   commands (`internal/api/pr_comment_commands.go:365`) and preflight
   (`internal/preflight/policy.go:17`). `docs/deploy-policies.md`
   matches this and says there is no MCP tool for policies because the
   tool list is at its token budget.
4. **The declarative schema is strict and self-describing.**
   `internal/spec/schema/app.schema.json` (about 28 KB) is embedded and
   compiled once (`internal/spec/schema.go:14`, `:28`) with
   `santhosh-tekuri/jsonschema/v6` (`go.mod`). Top level is `version`,
   `services`, `databases`, `additionalProperties: false`. A service
   has 30 properties, each with a `description`. `build.type` is an
   enum of `dockerfile`, `compose`, `railpack`, `static`, `image`.
   `databases` maps a name to `engine`, `version`, `backup` and preview
   flags. `levelrail-cli apps validate --file` runs `spec.Parse`
   (`cmd/levelrail-cli/apps_validate.go:59`).
5. **Environments and IAM already give scope and authority.**
   `store.Environment` has a `Protected` flag
   (`internal/store/environment.go:22`, `:26`). Four built-in
   environments exist; every environment has a kind (`dev`, `test`,
   `uat`, `production`, `preview`, `custom`) that IAM policies match on
   (`docs/environments.md`). Flat abilities are `read`,
   `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`
   (`internal/api/abilities.go:25` to `:30`), with optional IAM
   Allow/Deny statements on resources where an explicit Deny wins even
   over root (`docs/identity-and-access.md`, `internal/api/iam.go`).
6. **A v2 resource API with field ownership exists.**
   `GET/PATCH /api/v2/apps/{name}` (`internal/api/v2_apply.go`,
   `internal/api/v2_resources.go`) records which manager owns each spec
   leaf in `resource_managed_fields` (migration
   `3053_resource_managed_fields.sql`, `internal/store/resources_managed.go`).
7. **Nothing named app type exists.** A grep for `app_type`, `apptype`
   and `golden path` across `internal`, `cmd`, `docs` and `adr` finds
   only two unrelated test files.

So the pieces are all there (typed params, a strict schema, a scoped
policy gate, environment kinds, IAM), but no resource ties them into a
contract an admin owns and a developer picks.

## Decision (recommended)

Add one new resource, **AppType**, stored in the control plane, and one
optional field on an app, `type: <name>@<major>`. An app type is a
constraint plus defaults layered over the existing app spec. It never
generates code and never replaces the spec: the result of applying a
type is an ordinary `store.DesiredService`, so the reconciler, rollback
and promotion paths do not change.

### The resource

```yaml
name: internal-http-api
version: 3            # integer, bumped on publish; apps pin a major
title: Internal HTTP API
description: An HTTP service on the private network with a health check.
workload: web         # web | worker | cron | static (projecttpl kinds)
parameters:           # constrained JSON Schema subset, see below
  type: object
  required: [port]
  properties:
    port: {type: integer, minimum: 1024, maximum: 65535, default: 8080,
           description: Port the app listens on.}
    tier: {type: string, enum: [small, medium], default: small,
           description: Resource tier.}
environmentConfig:    # per environment kind, owned by admins, not apps
  production: {replicas: {minimum: 2}, health.readiness: required}
  dev:        {replicas: {maximum: 1}}
build:
  allowed: [railpack, dockerfile]   # subset of build.type enum
addons:
  databases: {allowed_engines: [postgres], max: 1}
  volumes: {allowed: false}
  domains: {allowed: true, suffixes: [".internal.example.com"]}
  cron: {allowed: false}
fixed:                # values the app may not set or change
  labels.team: required
  egress: private
defaults:             # applied when the app is silent
  resources.memory: 512Mi
messages:             # optional readable text per rule id
  build.allowed: "Internal APIs build with railpack or a Dockerfile."
```

**Parameter schema subset.** Only `type` (`string`, `integer`,
`boolean`, `object` one level deep), `enum`, `minimum`, `maximum`,
`minLength`, `maxLength`, `pattern` (RE2, length capped), `default`,
`description`, `required`, `additionalProperties: false` (always
forced). No `$ref`, no `oneOf`/`anyOf`/`allOf`, no `if`/`then`, no
remote ids, no formats beyond a `cron` marker reused from
`projecttpl.KindCron`. This is small enough to render as a dashboard
form, to explain in one error message per field, and to compile with
the schema library already in `go.mod`.

**Where parameters land.** Each parameter maps to exactly one spec path
through an explicit `binds` table (`port: services.*.port`,
`tier: resources` via a fixed lookup the type declares). No string
interpolation, no expressions. Unbound parameters are rejected at type
publish time.

**Per-environment config is separate.** `environmentConfig` is keyed by
environment kind, so it follows the same matching IAM already uses. It
can only tighten (minimum, maximum, required, fixed), never loosen what
`fixed` set. App authors never edit it; the app spec stays
environment-neutral and the layered env var merge from ADR 050 context
is untouched.

### How an app references a type

`type: internal-http-api@3` in `app.yaml`, or `type` on create via the
API and dashboard. The pin is a major version. Apps without `type`
behave exactly as today: no new checks, no new defaults. Adoption is
never forced; an admin can only make it a requirement with a new deploy
policy rule (`require_app_type`, see below), which uses the existing
`off`, `warn`, `enforce` modes and scopes.

### How violations surface

One evaluator, `apptype.Check(t AppType, envKind string, svc store.DesiredService) []policy.Violation`,
returns the same `Violation` shape (`rule`, `code`, `mode`, `message`,
`next_action`) with codes prefixed `apptype_` (for example
`apptype_build_not_allowed`, `apptype_param_invalid`,
`apptype_addon_not_allowed`, `apptype_fixed_value_changed`). It is
called from the four places that already evaluate deploy policy
(`policyGate`, scheduled deploys, PR comment commands, preflight), so
there is no new gate to bypass. Mode comes from the type's own
`enforcement` field, itself capped by the deploy policy rule
`app_type_conformance` at org, project or environment scope, so an
admin can run a new type in `warn` in dev and `enforce` in production.
The existing root override and `policy.override` app event apply
unchanged.

- `levelrail-cli apps validate --file app.yaml` resolves the type
  (locally cached or fetched) and prints the same violations.
- API: `POST /api/v1/app-types/{name}/check` with a spec body.
- Dashboard: the create flow reads the parameter schema and renders the
  form (enums as selects, integers with bounds, descriptions as help
  text). Locked fields render read-only with the reason.

### Versioning and migration

- A type has a draft and published versions. Publishing freezes the
  document and assigns the next integer. A version is immutable.
- An app pins `name@major`; a published change that only adds optional
  parameters or loosens limits is a minor and applies on the next
  deploy. A change that can break existing apps is a new major; apps
  stay on the old major until moved.
- Before publish, a dry run reports which pinned apps would fail, reusing
  the deploy policy dry run idea (`POST /api/v1/deploy-policies/check`).
- Adopting a type for an existing app is `levelrail-cli apps set-type`
  with a diff and a dry run. Removing a type returns the app to free
  form; its current values stay.
- Deleting a type with pinned apps is refused; deprecating it is
  allowed and shows a warning.

### RBAC

- Create, edit and publish a type: `root`, or an IAM Allow on
  `app-type:<name>` for a new action `apptype:publish`. Reading types
  needs `read`.
- Picking a type for an app needs `write` on that app. Changing an
  app's type major needs `write` plus `deploy` on the target
  environment.
- An explicit IAM Deny on `app-type:<name>` blocks both, consistent with
  how Deny already wins.

### CLI, API and MCP surface

- CLI: `levelrail-cli app-types list|get|apply -f|publish|check|deprecate`.
- API: `GET/POST /api/v1/app-types`, `GET/PUT /api/v1/app-types/{name}`,
  `POST /api/v1/app-types/{name}/publish`, `POST /api/v1/app-types/{name}/check`.
- MCP: one read tool, `app_types_list` with parameter schema, so an
  agent can propose a typed app. Writes stay off MCP. This must fit the
  tool token budget (`internal/mcptools/toolsets.go`); if it does not,
  it waits, as deploy policies did.

### Audit

Create, publish, deprecate and override each write an audit record with
actor, type name, version and a digest of the published document. Each
deploy attempt notes the type and version it was checked against, next
to the existing policy override note.

### Environment settings

Thresholds read from env, defaults in parentheses, names proposed:

- `LEVELRAIL_APPTYPE_ENABLED` (`off`): feature flag for the slices.
- `LEVELRAIL_APPTYPE_MAX_PARAMS` (`32`): parameters per type.
- `LEVELRAIL_APPTYPE_MAX_SCHEMA_BYTES` (`16384`): document size cap.
- `LEVELRAIL_APPTYPE_MAX_PATTERN_LEN` (`256`): regex length cap.
- `LEVELRAIL_APPTYPE_MAX_VERSIONS` (`50`): retained versions per type.

## Non-goals

- No templating language, no string interpolation, no expressions.
- No arbitrary code: no hooks, scripts or plugins in a type.
- No Kubernetes CRDs or controllers.
- No forced adoption; untyped apps keep today's behavior.
- No marketplace or cross-instance sharing of types.
- No replacement of the service template catalog or `init --template`.

## Alternatives rejected

- **Extend the service template catalog.** Templates are Compose bodies
  copied once with no back reference. Governance needs a live link to
  the app at every deploy.
- **Make `projecttpl` data driven.** It renders files once. Useful as
  the seed for built-in types (slice 4), not as the governing model.
- **Only add more deploy policy rules.** Policies are flat per scope
  and cannot carry typed parameters, defaults or a form. They stay the
  switch that turns conformance on.
- **Full JSON Schema for parameters.** Composition keywords make forms
  and error messages unpredictable, and broaden the parser surface.
- **A templating language over app.yaml.** Turns config review into
  code review and invites injection.

## Security

- Type documents are admin input but still parsed with size, parameter
  count and pattern length caps from env, and RE2 only, so a bad
  pattern cannot cause catastrophic backtracking.
- A type can only constrain or default existing spec fields. It cannot
  grant capabilities the app spec cannot already express, so it cannot
  widen privileges (privileged mode, host mounts and capabilities stay
  under deploy policy and ADR 049).
- `fixed` values are checked on every deploy path that evaluates policy,
  so editing the stored app through the v2 apply route does not dodge
  them.
- Secrets never appear in type documents. Parameters marked sensitive
  are rejected; secrets keep flowing through the existing env layers.

## Failure modes

- Type store unreachable at deploy: fail closed when conformance is
  `enforce`, warn and continue when `warn`. Rollbacks skip the check,
  like policy.
- Pinned major deleted or corrupt: deploy refused with
  `apptype_unresolved` and a next action to repin. Delete is refused
  while pins exist, so this needs manual database damage.
- Minor version breaks apps despite the dry run (data changed between
  dry run and publish): the violation appears at next deploy; the admin
  can roll the minor back by publishing the previous document.
- Dashboard form and server disagree: the server is authoritative; the
  form only uses the published schema.

## What cannot be verified locally

- Not verified: that every deploy path calls the policy evaluator. I
  found four call sites by grep; others (pipelines, promotion, imports)
  may skip it.
- Not verified: whether `projecttpl` is the only source of build
  method lists in the dashboard.
- Not verified: the MCP token headroom for one more read tool.
- Not verified: how `resource_managed_fields` would show fields a type
  owns; that the v2 apply path is used in practice.
- Not verified: the catalog count is from a grep of `ID:` in Go files,
  not a runtime count, and may include non-template structs.
- Performance of compiling many type schemas per deploy is unmeasured.

## Testing plan

- Unit: subset validator accepts allowed keywords and rejects each
  forbidden one with a readable message; parameter binding; tighten-only
  merge of `environmentConfig`; each `apptype_` code.
- Table test: an untyped app produces zero new violations (regression
  guard for "behaves as today").
- API: publish, pin, deploy in warn and enforce, override as root,
  denied publish via IAM Deny.
- One focused test per slice, no new cross-feature guard tests.

## Phases

1. Model, validator and `check` endpoint, no enforcement.
2. Deploy gate integration at the four call sites, `require_app_type`
   and `app_type_conformance` policy rules, audit.
3. Dashboard form generation and type admin pages.
4. Built-in types seeded from the five `projecttpl` templates, CLI
   `apps set-type`, MCP read tool.

## First thin slice (under 2 days)

Behind `LEVELRAIL_APPTYPE_ENABLED`: a new `internal/apptype` package
with the document struct, the constrained subset validator and
`Check` returning `policy.Violation`; a store table and migration for
types and published versions; `GET/POST /api/v1/app-types`, publish and
`check`; `levelrail-cli app-types apply|list|check`. No deploy gating,
no dashboard. Exit: a type can be published and an app spec checked
against it with readable messages.

## Slices and file ownership

| Slice | Owns | Touches (shared, sequence) |
| --- | --- | --- |
| 1 model + check | `internal/apptype/*`, new migration in `internal/store/migrations/`, `internal/store/app_type.go`, `internal/api/app_types.go`, `cmd/levelrail-cli/app_types.go` | `internal/api/router.go`, generated `openapi_gen.go` |
| 2 gate | `internal/api/deploy_apptype.go` | `internal/api/deploy_policy.go`, `scheduled_deploys.go`, `pr_comment_commands.go`, `internal/preflight/policy.go`, `internal/policy/policy.go` (new rule ids), `docs/deploy-policies.md` |
| 3 dashboard | new dashboard routes and form component | generated route tree |
| 4 seeds, CLI, MCP | `internal/apptype/builtin.go`, `internal/mcptools/tools_apptypes.go` | `internal/spec/schema/app.schema.json` (`type` field), `cmd/levelrail-cli/apps_validate.go` |

Slices 2 and 4 both touch policy and spec files; run them one after the
other, not in parallel.

## Consequences

- Admins get a governed golden path without a new language.
- One more resource to version, migrate and back up.
- Deploy policy grows two rules and becomes the single on switch.
- `app.schema.json` gains an optional top-level or per-service `type`
  field, a visible schema change for editors that use it.

## Open questions

1. Is `type` per service or per `app.yaml` file? Per service is more
   flexible; per file is simpler to explain.
2. Should `environmentConfig` key on environment kind only, or also
   allow a specific environment id?
3. Who can publish by default: root only, or a new built-in role?
4. Do built-in types ship enabled, or only as examples an admin copies?
5. Is a minor auto-applied on next deploy acceptable, or must apps
   always opt in to any new version?
6. Should conformance results be recorded for untyped apps too, as a
   report of what would fail if a type were required?
