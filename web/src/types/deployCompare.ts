// Wire type for GET /api/v1/apps/{name}/deploys/compare
// (internal/api/deploy_compare.go's handleCompareDeploys). IsCurrent true
// on a side means it's the app's current live desired state, not a
// stored attempt: DeployId is empty and CommitSha/Source/Status/
// StartedAt/FinishedAt carry no meaning there (DesiredService is live
// state, not a historical record).
export interface DeployCompareSide {
  deploy_id?: string
  is_current: boolean
  image: string
  commit_sha?: string
  source?: string
  status?: string
  started_at?: string
  finished_at?: string
}

export interface DeployCompareField {
  field: string
  from: string
  to: string
}

// unsnapshotted_fields and note are the honest limitation this feature's
// own design note requires: store.DeployAttempt never captured a
// per-attempt copy of env/resources/domains/etc, so those cannot be
// diffed across past deploys, only reported as not tracked.
export interface DeployCompare {
  service_name: string
  from: DeployCompareSide
  to: DeployCompareSide
  changes: DeployCompareField[]
  unsnapshotted_fields: string[]
  note: string
}
