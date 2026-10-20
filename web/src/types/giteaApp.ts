// Wire types for the Gitea App connection feature
// (internal/api/gitea_app.go, gitea_app_repos.go), matching their
// resource structs field for field.

export interface GiteaAppStatus {
  connected: boolean
  instance_url?: string
  client_id?: string
  created_at?: string
  authorized: boolean
  base_url?: string
}

// GiteaAppConnectRequest mirrors connectGiteaAppRequest
// (internal/api/gitea_app.go): the OAuth Application an operator
// registers by hand in their Gitea instance's Applications settings.
export interface GiteaAppConnectRequest {
  instance_url: string
  client_id: string
  client_secret: string
}

// GiteaAppRepo mirrors giteaAppRepoResource
// (internal/api/gitea_app_repos.go).
export interface GiteaAppRepo {
  full_name: string
  name: string
  private: boolean
  default_branch: string
  clone_url: string
  web_url: string
}

// GiteaAppBranch mirrors giteaAppBranchResource
// (internal/api/gitea_app_repos.go).
export interface GiteaAppBranch {
  name: string
  commit_sha: string
}

// GiteaAppUseRepoAsSourceRequest mirrors useGiteaRepoAsSourceRequest
// (internal/api/gitea_app_repos.go).
export interface GiteaAppUseRepoAsSourceRequest {
  app_name: string
  branch?: string
  build_type?: 'dockerfile' | 'railpack' | 'static'
  build_path?: string
  trigger_mode?: 'push' | 'release'
}
