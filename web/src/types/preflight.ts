// Wire types for POST /api/v1/apps/{name}/preflight and POST
// /api/v1/preflight (internal/api/preflight.go, internal/preflight.Report).
export type PreflightStatus = 'pass' | 'warn' | 'fail'

export interface PreflightCheck {
  id: string
  name: string
  status: PreflightStatus
  reason: string
  fix?: string
}

export interface PreflightReport {
  status: PreflightStatus
  checks: PreflightCheck[]
}

// Body for the not-yet-created-app route; every field is optional and an
// absent one skips the checks that depend on it.
export interface PreflightRequest {
  name?: string
  node_id?: string
  image?: string
  port?: number
  host_port?: number
  domains?: string[]
  memory_bytes?: number
  required_env?: string[]
  env_keys?: string[]
  bind_mounts?: string[]
  volume_paths?: string[]
  gpu?: boolean
  git_url?: string
  git_branch?: string
}
