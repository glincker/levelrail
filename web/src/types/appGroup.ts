import type { AppDetail, AppStatusSummary } from './appDetail'

// Mirrors internal/api's appGroupResource (internal/api/apps_group.go):
// GET /api/v1/apps/{name}/group's response. app_id carries `omitempty`
// on the Go side: empty means name has no store.App link yet (a service
// created before that concept existed), in which case services holds
// exactly one entry, name's own.
export interface AppGroup {
  app_id?: string
  services: AppDetail[]
  status: AppStatusSummary
}

// Mirrors internal/spec.Build's JSON wire shape as POST
// /api/v1/apps/{name}/deploy-spec expects it (internal/api/apps_multi.go's
// handleDeploySpec decodes straight into internal/spec.Service, so this
// deliberately does not reuse ServiceResources/ServiceHealth's
// bytes/nanosecond encoding from appDetail.ts, which is only valid for
// the general create/update endpoint).
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
