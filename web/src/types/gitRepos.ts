// Wire types for GET /api/v1/git/repos (internal/api/git_repo_list.go).

export type GitRepoProvider = 'github' | 'gitlab' | 'bitbucket' | 'gitea'

export type GitRepoMode = 'include' | 'exclude' | 'only'

export interface GitRepoItem {
  key: string
  full_name: string
  name: string
  owner: string
  private: boolean
  archived: boolean
  fork: boolean
  default_branch: string
  clone_url: string
  web_url?: string
  language?: string
  pushed_at?: string
  project_id?: number
  account_type?: 'user' | 'organization'
  /** Which connected account the row came from, so same-name repos stay apart. */
  provider?: GitRepoProvider
  connection_id?: string
  connection_label?: string
  account?: string
}

export interface GitRepoConnectionFacet {
  connection_id: string
  provider: GitRepoProvider
  label: string
  account?: string
  loaded: number
  failed: boolean
}

export interface GitRepoOwnerCount {
  login: string
  count: number
}

export interface GitRepoRateLimit {
  next_check_at: string
  retry_after_seconds: number
}

export interface GitRepoListResponse {
  /** Empty when the list merges every provider. */
  provider: GitRepoProvider | ''
  items: GitRepoItem[]
  total: number
  loaded: number
  limit: number
  offset: number
  next_offset: number | null
  has_more: boolean
  truncated: boolean
  cap: number
  cached_at: string
  stale: boolean
  refreshing: boolean
  owners: GitRepoOwnerCount[]
  errors?: {
    account_login: string
    provider?: GitRepoProvider
    connection_id?: string
    connection_label?: string
    error: string
  }[]
  /** Every account the merged list was built from. */
  connections: GitRepoConnectionFacet[]
  rate_limit?: GitRepoRateLimit
}

export interface GitRepoListParams {
  /** Narrow to one connected account (a connection id). */
  connection?: string
  search?: string
  owner?: string
  archived?: GitRepoMode
  forks?: GitRepoMode
  limit?: number
  offset?: number
  refresh?: boolean
}
