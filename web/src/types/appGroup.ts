import type { AppDetail, AppStatusSummary } from './appDetail'

// Empty app_id means name has no store.App link yet (a service created
// before that concept existed); services then holds exactly one entry.
export interface AppGroup {
  app_id?: string
  services: AppDetail[]
  status: AppStatusSummary
}

// Deliberately does not reuse appDetail.ts's bytes/nanosecond encoding;
// deploy-spec decodes straight into internal/spec.Service instead.
export type DeploySpecBuildType = 'dockerfile' | 'railpack' | 'static' | 'image'

export interface DeploySpecServiceInput {
  key: string
  buildType: DeploySpecBuildType
  buildPath?: string
  buildImage?: string
  port?: number
  domains?: string[]
  replicas?: number
}

export interface DeploySpecInput {
  repoUrl: string
  ref: string
  imageRepoBase?: string
  services: DeploySpecServiceInput[]
}

// Mirrors internal/api's deploySpecServiceResult (apps_multi.go): one
// service key's own outcome. error is set on failure, image on success,
// never both.
export interface DeploySpecServiceResult {
  service_key: string
  service_name: string
  image?: string
  error?: string
}

// Mirrors internal/api's deploySpecResponse. all_succeeded false means
// at least one service's own error is set; the request as a whole still
// succeeded at fanning out.
export interface DeploySpecResult {
  app_id: string
  services: DeploySpecServiceResult[]
  all_succeeded: boolean
}
