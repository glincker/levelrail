// Wire types for /api/v1/stacks (internal/api/stacks*.go).

import type { IacChange, IacSummary } from '../queries/iac'

export type StackRevisionStatus =
  | 'pending_approval'
  | 'applying'
  | 'applied'
  | 'partial'
  | 'failed'
  | 'superseded'

export type StackDriftPolicy = 'warn' | 'lock'

export interface StackLock {
  holder: string
  revision: number
  expires_at: string
}

export interface StackResource {
  kind: string
  key: string
  live_id: string
  since_revision: number
}

export type StackDriftStatus = 'none' | 'drifted'
export type StackDriftAction = 'changed' | 'missing' | 'owner_changed'

// Field paths only; the server never sends drifted values.
export interface StackDriftItem {
  kind: string
  name: string
  scope?: string
  action: StackDriftAction
  fields?: string[]
}

export interface StackDrift {
  status: StackDriftStatus
  checked_at?: string
  revision?: number
  summary: StackDriftItem[]
}

export interface Stack {
  name: string
  project_id: string
  project: string
  environment_id?: string
  allow_prune: boolean
  drift_policy: StackDriftPolicy
  pinned_revision?: number
  require_approval: boolean
  drift?: StackDrift
  created_at: string
  created_by: string
  latest_revision: number
  latest_status?: StackRevisionStatus
  lock?: StackLock
  resources?: StackResource[]
}

export interface StackList {
  stacks: Stack[]
}

export interface StackRevisionSummary {
  number: number
  status: StackRevisionStatus
  plan_hash: string
  rollback_of?: number
  idempotency_key?: string
  actor: string
  token_id?: string
  git_repo?: string
  git_ref?: string
  git_sha?: string
  ci_run_url?: string
  created_at: string
  finished_at?: string
  // A failed revision a reviewer turned down.
  rejected?: boolean
  // Not sent today; when present it wins over the client-side estimate.
  expires_at?: string
}

export interface StackHistory {
  stack: string
  pinned_revision?: number
  revisions: StackRevisionSummary[]
}

export interface StackFile {
  name: string
  content: string
}

export interface StackPlanBody {
  changes?: IacChange[]
  summary?: IacSummary
  hash?: string
}

export interface StackResultItem {
  kind?: string
  name?: string
  action?: string
  status?: string
  error?: string
}

export interface StackResultBody {
  summary?: IacSummary
  results?: StackResultItem[]
  [key: string]: unknown
}

export interface StackRevisionDetail extends StackRevisionSummary {
  files: StackFile[]
  plan?: StackPlanBody
  result?: StackResultBody
}

export type StackDiffAgainst = number | 'live'

export interface StackDiff {
  stack: string
  revision: number
  against: string | number
  plan_hash?: string
  changes: IacChange[]
}

export type StackDeleteMode = 'forget' | 'destroy'

export interface StackDeleteResult {
  stack: string
  mode: StackDeleteMode
  destroyed?: string[]
  forgotten?: string[]
  failed?: string[]
}

export interface StackSettingsInput {
  allow_prune?: boolean
  drift_policy?: StackDriftPolicy
  require_approval?: boolean
}

export interface StackRollbackResult {
  stack: string
  revision: StackRevisionSummary
  result?: StackResultBody
  replayed?: boolean
  warnings?: string[]
  // 202: the stack requires approval and nothing was applied yet.
  pending_approval?: boolean
  expires_at?: string
  plan?: StackPlanBody
}

export interface StackApproveInput {
  revision: number
  secrets?: Record<string, string>
  expectedPlanHash?: string
}

export type StackApproveResult = StackRollbackResult

export interface StackRejectInput {
  revision: number
  reason?: string
}

export interface StackRejectResult {
  stack: string
  revision: StackRevisionSummary
  reason?: string
}
