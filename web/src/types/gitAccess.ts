// GET /api/v1/git-connections/{id}/access. A capability with verified false
// was inferred from stored scopes rather than confirmed with the provider.
export type GitCapabilityId =
  | 'list_repos'
  | 'clone_sync'
  | 'register_webhook'
  | 'tag_push_trigger'
  | 'commit_status'
  | 'mr_comments'
  | 'deployments_report'
  | 'registry_pull'
  | 'import_group'

export interface GitCapabilityResult {
  id: GitCapabilityId
  state: 'allowed' | 'not_allowed' | 'needs_scope'
  needs_scope?: string
  verified: boolean
  note?: string
}

export interface GitAccessInfo {
  id: string
  provider: string
  instance_url: string
  account_name?: string
  auth_method: 'oauth_app' | 'token'
  scopes: string[]
  scopes_known: boolean
  scopes_source?: 'provider' | 'stored'
  expires_at?: string
  last_used_at?: string
  visible_accounts: { names: string[]; total: number; truncated: boolean }
  capabilities: GitCapabilityResult[]
  checked_at: string
  cached: boolean
  live_check_error?: string
}
