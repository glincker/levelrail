import type { ContainerExit } from './containerExit'
import type { DeployFailure } from './deployFailure'
import type { VulnCounts } from './supplyChain'

// Wire type for GET /api/v1/apps/{name}/deploy-attempts
// (internal/api/deploy_attempts.go's handleListDeployAttempts,
// deployAttemptResource). A real, row-per-attempt deploy history,
// additional to types/deploy.ts's ReconcileCondition ("current status",
// not history, see that file's own doc comment for why the two are
// deliberately kept separate rather than one endpoint's shape changing
// underneath its existing consumer).
// 'held' is an automatic deploy parked by a freeze window; 'superseded'
// never applied because a newer deploy already had; 'queued' waits behind
// another deploy (see wait_reason); 'canceled' was stopped by an operator
// before it wrote desired state.
export type DeployAttemptStatus =
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'held'
  | 'superseded'
  | 'queued'
  | 'canceled'
  | 'pending'

// Mirrors internal/store.DeployAttemptSource* on the wire: 'webhook' for
// an unattended git-push build, 'manual' for a dashboard-triggered
// git-source build, 'image' for a bare image-tag redeploy/rollback with
// no build step (and so no commit_sha), 'auto_rollback' for an
// unattended rollback triggered by internal/alerting.MaybeAutoRollback
// once a crashloop alert fires (also no commit_sha: it's the same
// image-tag path as 'image', just driven automatically).
export type DeployAttemptSource =
  | 'webhook'
  | 'manual'
  | 'image'
  | 'compose'
  | 'promote'
  | 'clone'
  | 'schedule'
  | 'auto_rollback'
  | 'restart'
  | 'start'
  | 'stop'
  | 'git_push'
  | 'poll'
  | 'upload'
  | 'pr_comment'

export interface DeployAttempt {
  /** 'held' is a finished build the app's publish lock kept off the live site. */
  publish_state?: 'held'
  id: string
  service_name: string
  image: string
  commit_sha?: string
  source?: DeployAttemptSource
  status: DeployAttemptStatus
  started_at: string
  finished_at?: string
  error?: string
  /** The create-app-from-git wizard's pre-flight detection result for
   *  this build, e.g. "Node.js". Absent when detection was skipped or
   *  found nothing buildable. */
  detected_framework?: string
  /** Push-payload metadata of a git build; absent for image deploys. */
  branch?: string
  commit_message?: string
  author?: string
  /** Content identity: registry digest, or a build's local image ID. */
  image_digest?: string
  /** Immutable address of this deploy while a retained copy exists. */
  deploy_url?: string
  /** How image_digest was obtained, e.g. Resolved or PullFailedUsingCached. */
  digest_reason?: string
  /** URL of this deploy's preview thumbnail, when one was captured. */
  preview_image_url?: string
  /** What the controller last saw running: 'serving' or 'mismatch'. */
  /** True for the one deploy serving the app now; set by the server. */
  live?: boolean
  /** Server classification, e.g. "rollback" for a redeploy of an earlier image. */
  trigger?: string
  rollout_state?: 'serving' | 'mismatch' | 'superseded'
  running_image_id?: string
  sequence?: number
  /** Why the attempt was held, superseded, or allowed through a freeze. */
  reason?: string
  queued_at?: string
  /** 1-based place in the app's queue, set only while queued. */
  queue_position?: number
  /** The attempt this one retried or rebuilt, absent when it is not a rerun. */
  rerun_of?: string
  /** 1-based position in the rerun chain, computed by the server. */
  chain_position?: number
  /** Why a queued or held deploy waits, e.g. "waiting for #dep_x". */
  wait_reason?: string
  /** The deploy a queued one waits for. */
  blocked_by?: string
  /** The newer queued deploy that replaced this one. */
  superseded_by?: string
  canceled_by?: string
  /** Packages in the deploy's SBOM, absent when none was recorded. */
  sbom_packages?: number
  /** Findings per severity, absent until the deploy was scanned. */
  vuln_counts?: VulnCounts
  /** Why the deploy failed or is held; absent otherwise. */
  failure?: DeployFailure
  /** Single-deploy endpoint only: how the release container stopped. */
  exit?: ContainerExit
}
