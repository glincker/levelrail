// Wire types for GitLab, Bitbucket and Gitea connections, many per provider
// (internal/api/git_connections.go). No credential ever appears here.

import type { GitConnectionStatus } from './gitProviders'

export type GitConnectionProvider =
  'gitlab' | 'bitbucket' | 'bitbucket_server' | 'gitea' | 'forgejo'

export interface GitProviderConnection {
  id: string
  provider: GitConnectionProvider
  label: string
  host: string
  instance_url: string
  client_id: string
  account_id?: string
  account_name?: string
  scopes?: string
  is_default: boolean
  authorized: boolean
  created_by?: string
  created_at: string
  source_count: number
  connection: GitConnectionStatus
  /** Browser navigation that authorizes or reauthorizes this connection. */
  authorize_path: string
  /** "token" for a pasted access token, "oauth" for an OAuth application. */
  auth_method?: 'token' | 'oauth'
}

export interface CreateGitConnectionRequest {
  provider: GitConnectionProvider
  instance_url?: string
  client_id?: string
  client_secret?: string
  label?: string
  /** Copies the OAuth application of this connection, so another account needs no second app. */
  reuse_connection_id?: string
  /** Access token used instead of an OAuth application, on any host. */
  token?: string
}

export interface GitConnectionBranch {
  name: string
  commit_sha: string
}

export interface UseRepoAsSourceRequest {
  app_name: string
  /** A project id for GitLab, owner/name for Bitbucket and Gitea. */
  repo: string
  branch?: string
  build_type?: 'dockerfile' | 'railpack' | 'static'
  build_path?: string
  trigger_mode?: 'push' | 'release'
}
