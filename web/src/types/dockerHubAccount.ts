// Wire types for GET /api/v1/registry-credentials/{id}/dockerhub and
// .../limits (internal/api/dockerhub_limits.go).

export interface DockerHubLimits {
  credential_id?: string
  authenticated: boolean
  known: boolean
  limit: number
  remaining: number
  window_seconds: number
  // "account" or "ip": how Docker Hub counted the pulls.
  source: string
  low: boolean
  warn_percent: number
  use_for_pulls: boolean
  checked_at: string
  cached: boolean
}

export interface DockerHubCapability {
  name: string
  status: string
}

export interface DockerHubStatus {
  credential_id?: string
  valid: boolean
  kind: string
  namespace: string
  message?: string
  scopes: string[] | null
  capabilities: DockerHubCapability[] | null
  private_repos_visible: boolean
  expiry_known: boolean
  expires_at: string | null
  permission_note: string
  limits: DockerHubLimits | null
  checked_at: string
  cached: boolean
}
