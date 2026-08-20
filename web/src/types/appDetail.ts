// Wire type for the app resource, GET/PUT /api/v1/apps/{name} and GET
// /api/v1/apps (internal/api/apps.go's appResource). Field names mirror
// the JSON wire shape directly (snake_case for ServiceResources,
// matching appResource's own struct tags) rather than being remapped to
// camelCase: this is an internal admin dashboard talking to one frozen
// backend contract, not a public SDK, and a remapping layer would just
// be one more place for the two shapes to silently drift apart.

export interface ServiceResources {
  memory_bytes?: number
  nano_cpus?: number
}

export interface ServiceProbe {
  path: string
  interval?: number
  timeout?: number
  failures?: number
}

export interface ServiceHealth {
  readiness?: ServiceProbe | null
  liveness?: ServiceProbe | null
}

// The three deploy strategy values internal/spec.Service.Strategy
// accepts (internal/spec/spec.go's StrategyRolling/StrategyRecreate/
// StrategyBlueGreen). "rolling" is included here because it's a real,
// readable value a service can already carry (e.g. a hand-edited
// app.yaml deployed outside this UI), not because it's meant to be
// offered as a new choice: internal/reconcile/application's controller
// refuses to run it (documented "unsupported" condition, not a bug),
// so DeployStrategyEditor deliberately does not let an operator pick it
// going forward, see that component's own comment.
export type DeployStrategy = 'rolling' | 'recreate' | 'blue-green'

// Matches internal/store's LogDrainType exactly: the two sink protocols
// internal/telemetry.DrainForwarder knows how to build.
export type LogDrainType = 'http' | 'syslog'

// Matches internal/store's LogDrain exactly (internal/api/apps.go's
// appResource.LogDrain, JSON-tagged the same way).
export interface LogDrain {
  type: LogDrainType
  target: string
  enabled: boolean
}

// Matches internal/api/apps.go's appResource exactly. `domains` and
// `env` carry `omitempty` on the Go side, so an app with neither set may
// omit them from the response entirely; callers should treat both as
// possibly undefined, not assume an empty array/object.
//
// node_id carries `omitempty` on the Go side and is response-only:
// appResource's own doc comment is explicit it is TASKS.md 3.3's
// placement field, set via PUT /api/v1/apps/{name}/node
// (handleSetAppNode) rather than through this type, the same
// response-only convention databaseDetail.ts's DatabaseResource
// documents for the equivalent field.
//
// strategy/replicas carry no `omitempty` on the Go side (appResource's
// own comment explains why: a real read is always the store's resolved,
// never-empty/zero value), so both are required here too, not optional.
export interface AppDetail {
  name: string
  image: string
  port: number
  // host_port pins the host-side port Docker binds `port` to
  // (internal/api/apps.go's appResource.HostPort). undefined/null means
  // "let Docker assign one", the ordinary case. Settable on create and
  // update, like `port` itself, not response-only.
  host_port?: number | null
  domains?: string[]
  env?: Record<string, string>
  resources?: ServiceResources | null
  health?: ServiceHealth | null
  strategy: DeployStrategy
  replicas: number
  // Custom Docker labels applied to the container at create time
  // (internal/api/apps.go's appResource.Labels), an escape hatch for
  // external tooling that keys off container labels. Carries
  // `omitempty` on the Go side, same optional-map convention `env`
  // already establishes above.
  labels?: Record<string, string>
  node_id?: string
  // project_id carries `omitempty` on the Go side and is response-only
  // on PUT (internal/api/apps.go's appResource own doc comment): set an
  // existing app's project via PUT /api/v1/apps/{name}/project
  // (useSetAppProject), the same node_id shape just above. The one
  // exception is POST /api/v1/apps (create): that handler accepts
  // project_id directly in the request body, see its own doc comment
  // for why a brand-new app is safe to assign a project to at create
  // time in a way an ordinary update is not.
  project_id?: string
  // environment_id carries `omitempty` on the Go side and is
  // response-only, the same node_id/project_id shape above: set via
  // PUT /api/v1/apps/{name}/environment (useSetAppEnvironment).
  environment_id?: string
  // storage_target_id carries `omitempty` on the Go side and is
  // response-only (internal/api/apps.go's appResource own doc comment):
  // which connected backup target (queries/backupTargets.ts) this app's
  // object-storage credentials resolve from, empty meaning none
  // attached. Set it via PUT/DELETE /api/v1/apps/{name}/storage
  // (useSetAppStorage/useClearAppStorage), the same node_id/project_id
  // shape just above.
  storage_target_id?: string
  // database_env carries `omitempty` on the Go side and is response-only
  // (internal/api/apps.go's appResource own doc comment): every env var
  // this app.yaml-deployed service resolves from a managed database
  // (internal/spec's own { from: "<database>.<field>" } syntax), keyed by
  // env var name. Read-only here on purpose: it comes from the deploy
  // pipeline, not this endpoint. See database_attachment below for the
  // settable, single-attachment counterpart.
  database_env?: Record<string, { database: string; field: string }>
  // database_attachment carries `omitempty` on the Go side and is
  // response-only, the same node_id/project_id/storage_target_id shape
  // above: which managed database this app resolves one connection env
  // var from, undefined meaning none attached. Set it via PUT/DELETE
  // /api/v1/apps/{name}/database (useSetAppDatabase/useClearAppDatabase).
  database_attachment?: {
    app_name?: string
    database_name: string
    env_var: string
    field: string
  }
  // suspended is response-only (internal/api/apps.go's appResource own
  // doc comment): set it via POST /api/v1/apps/{name}/stop and .../start
  // (useStopApp/useStartApp), the same node_id/project_id shape above.
  // No omitempty on the Go side (always present, never ambiguous).
  suspended: boolean
  // log_drain carries `omitempty` on the Go side and is response-only
  // (internal/api/apps.go's appResource own doc comment): forwards this
  // app's container logs to an external HTTP or syslog sink, additive
  // to the built-in node-local log store. undefined means none
  // configured. Set it via PUT/DELETE /api/v1/apps/{name}/log-drain
  // (useSetLogDrain/useClearLogDrain), the same node_id/project_id/
  // storage_target_id shape above.
  log_drain?: LogDrain | null
}

// GET /api/v1/apps' own wire shape (internal/api/apps.go's
// appListResource): AppDetail plus a batched status summary computed
// from one query across every listed app's conditions
// (store.GetConditionsForControllers), not present on the detail
// endpoint's AppDetail. Deliberately its own type rather than an
// optional field bolted onto AppDetail: GET /api/v1/apps/{name} never
// sends `status`, so callers of that endpoint should never see it typed
// as possibly-present.
export interface AppListEntry extends AppDetail {
  status: AppStatusSummary
}

// Mirrors internal/api/apps.go's appStatusSummary exactly. `variant`
// matches the same three categories web/src/lib/appStatus.ts's
// summarizeAppStatus already returns for the detail page's status
// badge, so a list-row dot and the detail-page badge for the same app
// read as the same status by construction, not by coincidence.
export interface AppStatusSummary {
  label: string
  variant: 'muted' | 'destructive' | 'success'
}
