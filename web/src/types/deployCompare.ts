import type { DeployStrategy } from './appDetail'

// Wire type for store.ServiceResources, reused as-is rather than
// redeclared: internal/api/deploy_compare.go's deployCompareSide embeds
// it directly.
export interface DeployCompareResources {
  memory_bytes?: number
  nano_cpus?: number
  swap_memory_bytes?: number
  cpuset_cpus?: string
}

// Wire type for store.ServiceProbe: interval/timeout are nanoseconds
// (time.Duration's default JSON encoding, no omitempty distinction from
// zero), the same convention appDetail.ts's own ServiceProbe already
// documents for the live app config.
export interface DeployCompareProbe {
  path: string
  interval?: number
  timeout?: number
  failures?: number
}

// Wire type for store.ServiceHealth, this side's readiness/liveness probe
// config at trigger time.
export interface DeployCompareHealth {
  readiness?: DeployCompareProbe | null
  liveness?: DeployCompareProbe | null
}

// Wire type for one of store.DeployAttemptSnapshot's Volumes: unlike
// appDetail.ts's AppVolume, name here is the resolved, platform-prefixed
// Docker volume name (store.ServiceVolume's own doc comment), not the
// logical app.yaml name, since a snapshot stores DesiredService.Volumes
// as-is rather than re-deriving the logical name.
export interface DeployCompareVolume {
  name: string
  container_path: string
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
// state, not a historical record). Port/host_port/domains/resources/env/
// health/replicas/strategy/volumes/labels are that side's config snapshot
// (store.DeployAttemptSnapshot): for a real attempt recorded before that
// snapshot existed (or before a given field was added to it), these are
// all empty/zero, not a real "port 0" or "no domains".
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
  health?: DeployCompareHealth | null
  replicas?: number
  strategy?: DeployStrategy
  volumes?: DeployCompareVolume[]
  labels?: Record<string, string>
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

// unsnapshotted_fields and note are the honest-limitation contract this
// feature's own design note requires. Every DesiredService field
// store.DeployAttempt tracks is now snapshotted per attempt, so
// unsnapshotted_fields is empty in practice today; it stays on the wire
// (not omitted) so a future field added without a matching snapshot
// update has somewhere to be listed.
export interface DeployCompare {
  service_name: string
  from: DeployCompareSide
  to: DeployCompareSide
  changes: DeployCompareField[]
  env_changes?: DeployCompareEnvChange[]
  unsnapshotted_fields: string[]
  note: string
}
