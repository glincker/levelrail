// Codes are frozen server side, so this list only grows. Anything missing falls back to the raw code.
const CODES = [
  'item_failed',
  'permission_denied',
  'unavailable',
  'read_failed',
  'protected_resource',
  'immutable_field',
  'synced_externally',
  'ref_project_missing',
  'ref_environment_missing',
  'ref_app_missing',
  'ref_secret_missing',
  'schema_unknown_field',
  'schema_version_unsupported',
  'ownership_conflict',
  'plan_stale',
  'plan_hash_version',
  'stack_locked',
  'stack_source_sync',
  'prune_guard',
  'apply_disabled',
  'invalid_request',
  'invalid_document',
  'invalid_name',
  'cron_schedule_invalid',
  'cron_job_invalid',
  'cron_http_invalid',
  'alert_rule_invalid',
  'approval_pending',
  'plan_superseded',
  'interrupted',
  'plan_job_not_found',
  'plan_timeout',
  'sync_repo_unreachable',
  'sync_path_invalid',
  'sync_limits_exceeded',
  'sync_plan_failed',
  'compose_path_invalid',
  'compose_ref_missing',
  'compose_too_large',
  'compose_too_many_files',
  'compose_mode_conflict',
  'compose_unsupported_key',
  'compose_rejected',
  'compose_secret_literal',
  'compose_var_missing',
  'compose_no_deploy_unsupported',
  'compose_apply_failed',
  'sync_secret_in_repo',
  'sync_disk_low',
  'sync_apply_failed',
  'environment_protected',
] as const

export const IAC_CODE_KEYS: Readonly<Record<string, string>> =
  Object.fromEntries(CODES.map((c) => [c, `iacPlan.codes.${c}`]))

export type IacPlanCode = (typeof CODES)[number]

// Narrowing keeps t() keys a literal union, so a missing locale entry fails tsc.
export function isIacPlanCode(code: string): code is IacPlanCode {
  return (CODES as readonly string[]).includes(code)
}
