// Wire type for the aggregated git provider capability summary,
// GET /api/v1/git-providers (internal/api/git_providers.go's
// gitProviderResource), matching its resource struct field for field.

export type GitProviderName = 'github' | 'gitlab' | 'bitbucket'

export interface GitProviderStatus {
  provider: GitProviderName
  connected: boolean
  can_list_branches: boolean
  can_register_webhook: boolean
  can_auth_clone: boolean
}
