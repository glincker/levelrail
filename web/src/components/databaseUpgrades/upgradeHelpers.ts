import type {
  AutoUpgradeLevel,
  UpgradeKind,
  UpgradePolicy,
  UpgradePolicyInput,
  UpgradeRunState,
  UpgradeSupport,
} from '../../types/databaseUpgrades'

export type BadgeVariant =
  'default' | 'outline' | 'destructive' | 'muted' | 'success' | 'warning'

export type WindowPreset = 'daily' | 'sundays' | 'custom'

export const PRESET_CRON: Record<Exclude<WindowPreset, 'custom'>, string> = {
  daily: '0 3 * * *',
  sundays: '0 3 * * 0',
}

export const RUN_STEPS: UpgradeRunState[] = [
  'pending',
  'backing_up',
  'upgrading',
  'verifying',
]

const SECONDS_PER_HOUR = 3600

export function presetForCron(cron: string): WindowPreset {
  const trimmed = cron.trim()
  if (trimmed === PRESET_CRON.daily) return 'daily'
  if (trimmed === PRESET_CRON.sundays) return 'sundays'
  return 'custom'
}

export function secondsToHours(seconds: number): number {
  return Math.round((seconds / SECONDS_PER_HOUR) * 100) / 100
}

export function hoursToSeconds(hours: number): number {
  return Math.round(hours * SECONDS_PER_HOUR)
}

export function supportVariant(support: UpgradeSupport): BadgeVariant {
  switch (support) {
    case 'supported':
      return 'success'
    case 'eol_soon':
      return 'warning'
    case 'eol':
      return 'destructive'
    default:
      return 'muted'
  }
}

export function kindVariant(kind: UpgradeKind): BadgeVariant {
  switch (kind) {
    case 'patch':
      return 'success'
    case 'minor':
      return 'default'
    default:
      return 'warning'
  }
}

export function stateVariant(state: UpgradeRunState): BadgeVariant {
  switch (state) {
    case 'succeeded':
      return 'success'
    case 'failed':
      return 'destructive'
    case 'reverted':
      return 'warning'
    default:
      return 'default'
  }
}

export interface PolicyDraft {
  autoUpgrade: AutoUpgradeLevel
  cron: string
  hours: string
  timezone: string
  verifyAfter: boolean
  revertOnFailure: boolean
  notify: string[]
}

export function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

export function policyToDraft(policy: UpgradePolicy): PolicyDraft {
  return {
    autoUpgrade: policy.auto_upgrade,
    cron: policy.window_cron,
    hours: String(secondsToHours(policy.window_duration_seconds)),
    timezone: policy.window_timezone || browserTimezone(),
    verifyAfter: policy.verify_after,
    revertOnFailure: policy.revert_on_failure,
    notify: policy.notify,
  }
}

export function draftToInput(draft: PolicyDraft): UpgradePolicyInput {
  return {
    auto_upgrade: draft.autoUpgrade,
    window_cron: draft.cron.trim(),
    window_duration_seconds: hoursToSeconds(
      Number.parseFloat(draft.hours) || 0,
    ),
    window_timezone: draft.timezone.trim(),
    verify_after: draft.verifyAfter,
    revert_on_failure: draft.revertOnFailure,
    notify: draft.notify,
  }
}

export function toggleChannel(ids: string[], id: string): string[] {
  return ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id]
}
