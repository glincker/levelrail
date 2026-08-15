// Wire type for the app resource, GET/PUT /api/v1/apps/{name} and GET
// /api/v1/apps (internal/api/apps.go's appResource): the list endpoint
// returns the same full shape as the detail endpoint, no separate
// summary projection, so this one type covers both routes. Field names
// mirror the JSON wire shape directly (snake_case for ServiceResources,
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
  domains?: string[]
  env?: Record<string, string>
  resources?: ServiceResources | null
  health?: ServiceHealth | null
  strategy: DeployStrategy
  replicas: number
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
}
