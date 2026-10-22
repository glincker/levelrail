import type { BackupStatus } from '../../../types/backupHistory'
import type { DrillSummary } from '../../../types/backupProtection'

export type SchedulePreset = 'daily' | 'weekly' | 'custom'

const PRESET_CRON: Record<Exclude<SchedulePreset, 'custom'>, string> = {
  daily: '0 3 * * *',
  weekly: '0 3 * * 0',
}

const CRON_FIELD_COUNT = 5

export function presetToCron(preset: SchedulePreset, custom: string): string {
  return preset === 'custom' ? custom.trim() : PRESET_CRON[preset]
}

export function isValidCron(expr: string): boolean {
  return expr.trim().split(/\s+/).length === CRON_FIELD_COUNT
}

export function parseRetain(raw: string): number | null {
  const n = Number(raw)
  return Number.isInteger(n) && n > 0 ? n : null
}

export type DrillState = 'none' | 'running' | 'passed' | 'failed'

export function drillState(drill: DrillSummary | undefined): DrillState {
  if (!drill) return 'none'
  if (drill.status === 'passed') return 'passed'
  if (drill.status === 'failed') return 'failed'
  return 'running'
}

export function canSubmitStep(
  step: 'target' | 'schedule' | 'retention',
  v: { targetId: string; cron: string; retain: string },
): boolean {
  if (step === 'target') return v.targetId !== ''
  if (step === 'schedule') return isValidCron(v.cron)
  return parseRetain(v.retain) !== null
}

export function isTerminal(status: BackupStatus | undefined): boolean {
  return status === 'succeeded' || status === 'failed'
}
