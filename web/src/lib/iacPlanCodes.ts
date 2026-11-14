// Codes are frozen server side, so this map only grows. Anything missing falls back to the raw code.
const CODES = [
  'item_failed',
  'permission_denied',
  'protected_resource',
  'immutable_field',
  'synced_externally',
  'ref_project_missing',
  'ref_environment_missing',
  'ref_app_missing',
  'ref_secret_missing',
  'schema_unknown_field',
  'plan_stale',
  'stack_locked',
  'prune_guard',
  'invalid_document',
  'invalid_name',
  'cron_schedule_invalid',
  'cron_job_invalid',
  'cron_http_invalid',
  'alert_rule_invalid',
  'environment_protected',
] as const

export const IAC_CODE_KEYS: Readonly<Record<string, string>> =
  Object.fromEntries(CODES.map((c) => [c, `iacPlan.codes.${c}`]))
