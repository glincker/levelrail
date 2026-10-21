// Wire types for GET/PUT /api/v1/apps/{name}/branch-deploys
// (internal/api/branch_deploys_handlers.go).

export type BranchDeployMode = 'off' | 'all' | 'patterns'

export type BranchDeployStatus = 'deploying' | 'active' | 'failed'

export interface BranchDeploySettings {
  mode: BranchDeployMode
  patterns: string[]
  production_branch: string
  idle_ttl_hours: number
}

export interface BranchDeploy {
  branch: string
  slug: string
  deploy_app_id: string
  url?: string
  domain?: string
  head_sha: string
  head_message?: string
  status: BranchDeployStatus
  status_reason?: string
  created_at: string
  updated_at: string
  expires_at?: string
}

export interface BranchDeploys {
  settings: BranchDeploySettings
  deploys: BranchDeploy[]
}

export interface BranchDeploySettingsUpdate {
  mode: BranchDeployMode
  patterns: string[]
}
