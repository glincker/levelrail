// Wire type for the aggregated git provider capability summary,
// GET /api/v1/git-providers (internal/api/git_providers.go's
// gitProviderResource), matching its resource struct field for field.

export type GitProviderName = 'github' | 'gitlab' | 'bitbucket' | 'gitea'

export type GitConnectionHealth =
  'ok' | 'needs_reauthorize' | 'revoked' | 'rate_limited'

export interface GitConnectionStatus {
  status: GitConnectionHealth
  detail?: string
  next_check_at?: string
}

export interface GitProviderStatus {
  provider: GitProviderName
  connected: boolean
  can_list_branches: boolean
  can_register_webhook: boolean
  can_auth_clone: boolean
  /** Absent on an older control plane; treated as ok. */
  connection?: GitConnectionStatus
}

// DELETE /api/v1/{provider}-app: what the disconnect did on the provider's side.
export interface GitDisconnectResult {
  disconnected: boolean
  token_revoked: boolean
  note: string
}
