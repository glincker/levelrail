import type { EnvironmentResource } from './environment'

// Wire type for one resolved key inside EnvironmentCompareSide.env
// (internal/api/environment_compare.go's environmentEnvEntryResource).
// value is only ever populated when secret is false: a secret-marked
// key's plaintext is never resolved by this endpoint in the first
// place, so there is nothing to redact beyond an absent field.
export interface EnvironmentEnvEntry {
  key: string
  value?: string
  secret: boolean
}

// Wire type for GET .../environments/compare's own env list side
// (internal/api/environment_compare.go's environmentCompareSide): one
// environment's resolved effective env vars (organization, then
// project, then this environment's own vars, same precedence
// internal/reconcile/application's resolveEnv itself applies, stopping
// short of any single service's own env).
export interface EnvironmentCompareSide {
  environment: EnvironmentResource
  env: EnvironmentEnvEntry[]
}

// status is one of:
//  - "only_in_a" / "only_in_b": the other side has nothing for this key.
//  - "changed": a plain key both sides have, with different values.
//  - "masked": a key both sides have where at least one side marks it
//    secret; this control plane cannot tell whether the two values
//    differ without decrypting them, so it never guesses "changed" or
//    "same" here.
// a/b are only ever populated for a non-secret key (internal/api's
// environmentEnvDiffEntry doc comment).
export type EnvironmentEnvDiffStatus =
  'only_in_a' | 'only_in_b' | 'changed' | 'masked'

export interface EnvironmentEnvDiffEntry {
  key: string
  secret: boolean
  status: EnvironmentEnvDiffStatus
  a?: string
  b?: string
}

// Wire type for GET /api/v1/projects/{id}/environments/compare
// (internal/api/environment_compare.go's handleCompareEnvironmentEnv).
export interface EnvironmentCompare {
  project_id: string
  a: EnvironmentCompareSide
  b: EnvironmentCompareSide
  diff: EnvironmentEnvDiffEntry[]
  note: string
}
