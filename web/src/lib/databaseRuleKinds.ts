import type { TFunction } from 'i18next'

export const DATABASE_RULE_KINDS = [
  'database_disk_high',
  'database_connections_high',
  'database_down',
  'database_backup_stale',
] as const

export function isDatabaseRuleKind(kind: string): boolean {
  return (DATABASE_RULE_KINDS as readonly string[]).includes(kind)
}

export function databaseKindLabel(t: TFunction<'observability'>, kind: string) {
  switch (kind) {
    case 'database_disk_high':
      return t('databaseRule.kind.database_disk_high')
    case 'database_connections_high':
      return t('databaseRule.kind.database_connections_high')
    case 'database_down':
      return t('databaseRule.kind.database_down')
    default:
      return t('databaseRule.kind.database_backup_stale')
  }
}

export function databaseHint(t: TFunction<'observability'>, kind: string) {
  switch (kind) {
    case 'database_disk_high':
      return t('databaseRule.hint.database_disk_high')
    case 'database_connections_high':
      return t('databaseRule.hint.database_connections_high')
    case 'database_down':
      return t('databaseRule.hint.database_down')
    default:
      return t('databaseRule.hint.database_backup_stale')
  }
}

export interface DatabaseRuleFormShape {
  backupDatabaseName: string
  threshold: number
}
