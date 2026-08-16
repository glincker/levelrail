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
  // base_url is what handleStartGitHubAppRegistration's manifest and
  // webhook/callback/setup URLs will actually point at (derived from
  // ingress settings' primary domain), empty when no primary domain is
  // configured yet. Shown as a disclaimer before the automated flow, so
  // connecting never surprises an operator with a callback pointing
  // somewhere other than where they're actually running this instance.
  base_url?: string
}

// GitHubAppManualConnectRequest mirrors manualGitHubAppRequest
// (internal/api/github_app_manual.go): every field GitHub's own App
// settings page shows for an App created by hand, typed in instead of
// exchanged via the manifest flow.
export interface GitHubAppManualConnectRequest {
  app_id: number
  client_id: string
  client_secret: string
  webhook_secret: string
  private_key: string
  installation_id?: number
  account_login?: string
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
