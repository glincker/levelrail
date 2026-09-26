import type { ActivityEvent } from '../../queries/activity'
import type { FailedDeploy } from '../../queries/failedDeploys'
import type { DeployApprovalResource } from '../../types/deployApproval'
import { formatAge } from '../../lib/format'

export type NotificationGroup = 'deploys' | 'alerts' | 'approvals' | 'system'

export const GROUP_ORDER: NotificationGroup[] = [
  'approvals',
  'deploys',
  'alerts',
  'system',
]

export const GROUP_LABELS: Record<NotificationGroup, string> = {
  approvals: 'Approvals',
  deploys: 'Deploys',
  alerts: 'Alerts',
  system: 'System',
}

export interface ShellNotification {
  id: string
  group: NotificationGroup
  severity: 'critical' | 'warning' | 'info'
  title: string
  detail: string
  href: string
}

export function buildNotifications(src: {
  failedDeploys?: FailedDeploy[]
  approvals?: DeployApprovalResource[]
  activity?: ActivityEvent[]
}): ShellNotification[] {
  const out: ShellNotification[] = []
  for (const a of src.approvals ?? []) {
    out.push({
      id: `approval:${a.id}`,
      group: 'approvals',
      severity: 'warning',
      title: `${a.service_name} awaits approval`,
      detail: `Requested by ${a.requested_by_name || a.requested_by}`,
      href: '/approvals',
    })
  }
  for (const d of src.failedDeploys ?? []) {
    out.push({
      id: `deploy:${d.id}`,
      group: 'deploys',
      severity: 'critical',
      title: `Deploy of ${d.service_name} failed`,
      detail: formatAge(d.finished_at ?? d.started_at),
      href: `/apps/${d.service_name}/deploys`,
    })
  }
  for (const e of src.activity ?? []) {
    out.push({
      id: `system:${e.id}`,
      group: 'system',
      severity: e.severity,
      title: e.title,
      detail: e.detail,
      href: e.href,
    })
  }
  return out
}

export function groupNotifications(
  list: ShellNotification[],
): { group: NotificationGroup; label: string; items: ShellNotification[] }[] {
  return GROUP_ORDER.flatMap((group) => {
    const items = list.filter((n) => n.group === group)
    return items.length > 0
      ? [{ group, label: GROUP_LABELS[group], items }]
      : []
  })
}

export function unreadCount(
  list: ShellNotification[],
  read: ReadonlySet<string>,
): number {
  return list.filter((n) => !read.has(n.id)).length
}

const MAX_READ_IDS = 200

export function markAllRead(
  read: ReadonlySet<string>,
  list: ShellNotification[],
): string[] {
  return [...new Set([...list.map((n) => n.id), ...read])].slice(
    0,
    MAX_READ_IDS,
  )
}
