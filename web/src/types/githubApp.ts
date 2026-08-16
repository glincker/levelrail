// Wire types for the GitHub App connection feature
// (internal/api/github_app.go, github_app_repos.go), matching their
// resource structs field for field.

// GitHubAppStatus mirrors gitHubAppStatusResource
// (internal/api/github_app.go). Deliberately no secret fields: the
// backend never echoes client_secret/webhook_secret/private_key back in
// any response body, so there is nothing to type here for them.
export interface GitHubAppStatus {
  connected: boolean
  app_id?: number
  client_id?: string
  installed: boolean
  account_login?: string
  created_at?: string
}

// GitHubAppRepo mirrors gitHubAppRepoResource
// (internal/api/github_app_repos.go).
export interface GitHubAppRepo {
  full_name: string
  name: string
  owner_login: string
  private: boolean
  default_branch: string
  clone_url: string
}

// GitHubAppBranch mirrors gitHubAppBranchResource
// (internal/api/github_app_repos.go).
export interface GitHubAppBranch {
  name: string
  commit_sha: string
}
