import type { DefaultAlert } from '../queries/alertDefaults'
import type { AlertHistoryEntry } from '../types/alertNoise'
import type { RuntimeSetting } from '../types/runtimeSettings'

// Rule kinds the engine fans out over every managed database.
export const DATABASE_HEALTH_KINDS = [
  'database_crash_loop',
  'database_disk_growth',
  'database_connections_saturated',
  'database_backup_missing',
] as const

export type DatabaseHealthKind = (typeof DATABASE_HEALTH_KINDS)[number]

export function isDatabaseHealthKind(kind: string): kind is DatabaseHealthKind {
  return (DATABASE_HEALTH_KINDS as readonly string[]).includes(kind)
}

export type DatabaseAlertState = 'ok' | 'firing' | 'off'

export interface DatabaseAlertRow {
  id: string
  kind: string
  name: string
  severity: string
  state: DatabaseAlertState
  // Newest fired event for the rule, when the history has one.
  since?: string
  // Notice text of that event, shown as the value when it names this database.
  detail?: string
  // True when the rule fires but nothing ties it to this database yet.
  unattributed?: boolean
}

// The alert API reports health rules platform wide, so a firing rule is tied
// to one database by the name in its newest fired event, client side.
export function deriveDatabaseAlerts(
  defaults: DefaultAlert[],
  history: AlertHistoryEntry[],
  database: string,
): DatabaseAlertRow[] {
  const newest = (ruleId: string) =>
    history
      .filter((h) => h.rule_id === ruleId && h.event === 'fired')
      .sort((a, b) => b.at.localeCompare(a.at))[0]
  return defaults
    .filter((d) => isDatabaseHealthKind(d.kind))
    .map((d): DatabaseAlertRow => {
      const base = {
        id: d.id,
        kind: d.kind,
        name: d.name,
        severity: d.severity,
      }
      if (!d.enabled) return { ...base, state: 'off' }
      if (!d.firing) return { ...base, state: 'ok' }
      const fired = newest(d.id)
      if (fired?.detail && !fired.detail.includes(database)) {
        return { ...base, state: 'ok' }
      }
      return {
        ...base,
        state: 'firing',
        since: fired?.at,
        detail: fired?.detail,
        unattributed: !fired?.detail,
      }
    })
}

const SETTING_PREFIX = 'alerts.database.'

function settingValue(settings: RuntimeSetting[], key: string): string {
  const found = settings.find((s) => s.key === SETTING_PREFIX + key)
  return found === undefined ? '' : String(found.value)
}

export interface ThresholdChip {
  id: string
  value: string
}

// Chip ids map to dbMonitoring alerts.chips.* strings.
export function thresholdChips(
  kind: string,
  severity: string,
  settings: RuntimeSetting[],
): ThresholdChip[] {
  const pick = (id: string, key: string): ThresholdChip[] => {
    const value = settingValue(settings, key)
    return value === '' || value === '0' ? [] : [{ id, value }]
  }
  switch (kind) {
    case 'database_disk_growth':
      return [
        ...(severity === 'critical'
          ? pick('diskCritical', 'disk_growth_critical_days')
          : pick('diskWarn', 'disk_growth_warn_days')),
        ...pick('holdFor', 'disk_growth_for'),
      ]
    case 'database_connections_saturated':
      return [
        ...pick('connections', 'connections_percent'),
        ...pick('holdFor', 'connections_for'),
      ]
    case 'database_backup_missing':
      return [
        ...pick('backupWindow', 'backup_missing_window'),
        ...pick('unprotected', 'backup_unprotected_mb'),
      ]
    default:
      return []
  }
}
