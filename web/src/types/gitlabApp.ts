// Wire types for the GitLab App connection feature
// (internal/api/gitlab_app.go, gitlab_app_projects.go), matching their
// resource structs field for field.

export interface GitLabAppStatus {
  connected: boolean
  instance_url?: string
  client_id?: string
  created_at?: string
  authorized: boolean
  base_url?: string
}

// GitLabAppConnectRequest mirrors connectGitLabAppRequest
// (internal/api/gitlab_app.go): the OAuth Application an operator
// registers by hand in their GitLab instance's Applications settings.
export interface GitLabAppConnectRequest {
  instance_url: string
  client_id: string
  client_secret: string
}

// GitLabAppProject mirrors gitLabAppProjectResource
// (internal/api/gitlab_app_projects.go).
export interface GitLabAppProject {
  id: number
  name: string
  path_with_namespace: string
  clone_url: string
  default_branch: string
  visibility: string
  web_url: string
}

// GitLabAppUseProjectAsSourceRequest mirrors
// useGitLabProjectAsSourceRequest (internal/api/gitlab_app_projects.go).
export interface GitLabAppUseProjectAsSourceRequest {
  app_name: string
  branch?: string
  build_type?: 'dockerfile' | 'railpack' | 'static'
  build_path?: string
}
