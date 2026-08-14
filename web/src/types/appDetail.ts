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
export interface AppDetail {
  name: string
  image: string
  port: number
  domains?: string[]
  env?: Record<string, string>
  resources?: ServiceResources | null
  health?: ServiceHealth | null
  node_id?: string
}
