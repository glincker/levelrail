// Wire type for the aggregated git provider capability summary,
// GET /api/v1/git-providers (internal/api/git_providers.go's
// gitProviderResource), matching its resource struct field for field.

export type GitProviderName =
  'github' | 'gitlab' | 'bitbucket' | 'bitbucket_server' | 'gitea' | 'forgejo'

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
  /** Worst recorded health among the provider's connections. */
  connection?: GitConnectionStatus
  /** Configured connections, and how many of them hold a token. */
  connections?: number
  authorized?: number
}

// DELETE /api/v1/git-connections/{id} and DELETE /api/v1/github-app: what the
// disconnect did on the provider's side.
export interface GitDisconnectResult {
  disconnected: boolean
  token_revoked: boolean
  note: string
  /** Git sources that used the connection; they stay and need a reconnect. */
  affected_sources?: string[]
}
