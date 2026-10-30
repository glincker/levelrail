import type { TFunction } from 'i18next'

// Locale keys (observability.json, alertKinds) for each alert kind that
// needs a friendly label, description and "what to do" hint.
export const ALERT_KIND_COPY = {
  backup_failed: {
    label: 'alertKinds.backup_failed.label',
    description: 'alertKinds.backup_failed.description',
    action: 'alertKinds.backup_failed.action',
  },
  deploy_failed: {
    label: 'alertKinds.deploy_failed.label',
    description: 'alertKinds.deploy_failed.description',
    action: 'alertKinds.deploy_failed.action',
  },
  monitor_down: {
    label: 'alertKinds.monitor_down.label',
    description: 'alertKinds.monitor_down.description',
    action: 'alertKinds.monitor_down.action',
  },
  database_down: {
    label: 'alertKinds.database_down.label',
    description: 'alertKinds.database_down.description',
    action: 'alertKinds.database_down.action',
  },
  database_backup_stale: {
    label: 'alertKinds.database_backup_stale.label',
    description: 'alertKinds.database_backup_stale.description',
    action: 'alertKinds.database_backup_stale.action',
  },
  database_disk_high: {
    label: 'alertKinds.database_disk_high.label',
    description: 'alertKinds.database_disk_high.description',
    action: 'alertKinds.database_disk_high.action',
  },
  database_connections_high: {
    label: 'alertKinds.database_connections_high.label',
    description: 'alertKinds.database_connections_high.description',
    action: 'alertKinds.database_connections_high.action',
  },
  node_disk_space: {
    label: 'alertKinds.node_disk_space.label',
    description: 'alertKinds.node_disk_space.description',
    action: 'alertKinds.node_disk_space.action',
  },
  email_delivery_failing: {
    label: 'alertKinds.email_delivery_failing.label',
    description: 'alertKinds.email_delivery_failing.description',
    action: 'alertKinds.email_delivery_failing.action',
  },
} as const

export type AlertKindCopyKind = keyof typeof ALERT_KIND_COPY

export function isAlertKindCopyKind(kind: string): kind is AlertKindCopyKind {
  return Object.prototype.hasOwnProperty.call(ALERT_KIND_COPY, kind)
}

// Unknown kinds fall back to the raw kind string, so new backend kinds
// still render something readable before their copy ships.
export function alertKindLabel(
  t: TFunction<'observability'>,
  kind: string,
): string {
  if (!isAlertKindCopyKind(kind)) return kind
  return t(ALERT_KIND_COPY[kind].label)
}
