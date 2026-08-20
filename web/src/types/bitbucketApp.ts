// Wire types for the Bitbucket App connection feature
// (internal/api/bitbucket_app.go, bitbucket_app_repos.go), matching
// their resource structs field for field.

export interface BitbucketAppStatus {
  connected: boolean
  key?: string
  created_at?: string
  authorized: boolean
  base_url?: string
}

// BitbucketAppConnectRequest mirrors connectBitbucketAppRequest
// (internal/api/bitbucket_app.go): the OAuth consumer an operator
// registers by hand in their Bitbucket workspace settings.
export interface BitbucketAppConnectRequest {
  key: string
  secret: string
}

// BitbucketAppRepo mirrors bitbucketAppRepoResource
// (internal/api/bitbucket_app_repos.go).
export interface BitbucketAppRepo {
  full_name: string
  name: string
  private: boolean
  default_branch: string
  clone_url: string
  web_url: string
}

// BitbucketAppBranch mirrors bitbucketAppBranchResource
// (internal/api/bitbucket_app_repos.go).
export interface BitbucketAppBranch {
  name: string
  commit_sha: string
}

// BitbucketAppUseRepoAsSourceRequest mirrors
// useBitbucketRepoAsSourceRequest (internal/api/bitbucket_app_repos.go).
export interface BitbucketAppUseRepoAsSourceRequest {
  app_name: string
  branch?: string
  build_type?: 'dockerfile' | 'railpack' | 'static'
  build_path?: string
}
