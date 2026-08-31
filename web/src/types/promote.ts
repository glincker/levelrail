// Wire types for GET /api/v1/apps/{name}/promote/preview and POST
// /api/v1/apps/{name}/promote (internal/api/promote.go). PromotePreviewSide
// mirrors deployCompare.ts's DeployCompareSide shape but always carries
// live desired state, not a stored deploy attempt: there is no
// deploy_id/commit_sha/status/timestamps to show for either side.

import type { EnvironmentResource } from './environment'
import type { DeployCompareField } from './deployCompare'

export interface PromotePreviewSide {
  app_name: string
  image: string
}

export interface PromotePreviewResource {
  source_app: string
  target_app: string
  environment: EnvironmentResource
  from: PromotePreviewSide
  to: PromotePreviewSide
  changes: DeployCompareField[]
  unsnapshotted_fields: string[]
  note: string
}
