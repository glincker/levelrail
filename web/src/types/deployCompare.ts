// Wire type for store.ServiceResources, reused as-is rather than
// redeclared: internal/api/deploy_compare.go's deployCompareSide embeds
// it directly.
export interface DeployCompareResources {
  memory_bytes?: number
  nano_cpus?: number
  swap_memory_bytes?: number
  cpuset_cpus?: string
}

// Wire type for store.DeployAttemptEnvKey: one env var key captured in a
// deploy attempt's config snapshot. value is only ever populated when
// kind is "literal": a "secret" or "database" key never carries one, see
// internal/store/deploy_attempt.go's DeployAttemptEnvKey doc comment.
export interface DeployCompareEnvKey {
  key: string
  kind: 'literal' | 'secret' | 'database'
  value?: string
}

// Wire type for GET /api/v1/apps/{name}/deploys/compare
// (internal/api/deploy_compare.go's handleCompareDeploys). IsCurrent true
// on a side means it's the app's current live desired state, not a
// stored attempt: DeployId is empty and CommitSha/Source/Status/
// StartedAt/FinishedAt carry no meaning there (DesiredService is live
// state, not a historical record). Port/host_port/domains/resources/env
// are that side's config snapshot (store.DeployAttemptSnapshot): for a
// real attempt recorded before that snapshot existed, these are all
// empty/zero, not a real "port 0" or "no domains".
export interface DeployCompareSide {
  deploy_id?: string
  is_current: boolean
  image: string
  commit_sha?: string
  source?: string
  status?: string
  started_at?: string
  finished_at?: string
  port?: number
  host_port?: number
  domains?: string[]
  resources?: DeployCompareResources
  env?: DeployCompareEnvKey[]
}

export interface DeployCompareField {
  field: string
  from: string
  to: string
}

// Wire type for one deployCompareEnvChange
// (internal/api/deploy_compare.go): status is "added", "removed", or
// "changed". from/to are only ever populated when kind is "literal", the
// same rule DeployCompareEnvKey.value follows and for the same reason: a
// secret- or database-backed key's value is never known to this control
// plane in the first place.
export interface DeployCompareEnvChange {
  key: string
  kind: 'literal' | 'secret' | 'database'
  status: 'added' | 'removed' | 'changed'
  from?: string
  to?: string
}

// unsnapshotted_fields and note are the honest limitation this feature's
// own design note requires: store.DeployAttempt still doesn't capture a
// per-attempt copy of health checks, replica count, strategy, volumes, or
// labels, so those cannot be diffed across past deploys, only reported as
// not tracked.
export interface DeployCompare {
  service_name: string
  from: DeployCompareSide
  to: DeployCompareSide
  changes: DeployCompareField[]
  env_changes?: DeployCompareEnvChange[]
  unsnapshotted_fields: string[]
  note: string
}
