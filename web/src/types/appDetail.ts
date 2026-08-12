// Wire types for the single-app resource, GET/PUT /api/v1/apps/{name}
// (internal/api/apps.go's appResource). Deliberately separate from
// types/app.ts's list-page AppSummary/AppPage: those are summary rows
// for the virtualized list, this is the full resource the detail route
// reads and writes. Field names below mirror the JSON wire shape
// directly (snake_case for ServiceResources, matching appResource's own
// struct tags) rather than being remapped to camelCase: this is an
// internal admin dashboard talking to one frozen backend contract, not a
// public SDK, and a remapping layer would just be one more place for the
// two shapes to silently drift apart.

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
export interface AppDetail {
  name: string
  image: string
  port: number
  domains?: string[]
  env?: Record<string, string>
  resources?: ServiceResources | null
  health?: ServiceHealth | null
}
