import type { FailedDeploy } from '../queries/failedDeploys'
import type { AttentionFeedItem } from '../queries/attentionFeed'
import type { InboxItem, InboxState } from '../queries/inbox'
import type { SignInApprovalRequest } from '../queries/signIn'
import type { DeployApprovalResource } from '../types/deployApproval'

export type InboxKind = 'alert' | 'approval' | 'deploy' | 'upgrade'

export const INBOX_KINDS: InboxKind[] = [
  'alert',
  'approval',
  'deploy',
  'upgrade',
]

export type InboxAction =
  | { type: 'deployApproval'; approvalId: string }
  | { type: 'signIn'; approvalId: string }
  | { type: 'retryDeploy'; app: string; image: string }
  | { type: 'link'; to: string }

export interface InboxEntry {
  id: string
  kind: InboxKind
  severity: 'critical' | 'warning' | 'info'
  title: string
  detail: string
  at?: string
  href: string
  read: boolean
  archived: boolean
  deletable: boolean
  resolved: boolean
  groupKey?: string
  action?: InboxAction
}

export interface InboxSources {
  alerts?: InboxItem[]
  failedDeploys?: FailedDeploy[]
  approvals?: DeployApprovalResource[]
  signIns?: SignInApprovalRequest[]
  feed?: AttentionFeedItem[]
  state?: InboxState
  t: (key: string, opts?: Record<string, string>) => string
}

// Feed kinds that belong in the inbox; ids match internal/attention's kind:subject.
const FEED_KINDS: Record<string, InboxKind> = {
  db_upgrade_failed: 'upgrade',
}

export function buildInboxEntries(src: InboxSources): InboxEntry[] {
  const read = new Set(src.state?.read_ids ?? [])
  const archived = new Set(src.state?.archived_ids ?? [])
  const out: InboxEntry[] = []
  const mark = (
    e: Omit<InboxEntry, 'read' | 'archived' | 'deletable' | 'resolved'> & {
      resolved?: boolean
    },
    isRead = false,
  ) =>
    out.push({
      ...e,
      resolved: e.resolved ?? false,
      read: isRead || read.has(e.id),
      archived: archived.has(e.id),
      deletable: e.kind === 'alert',
    })

  for (const a of src.alerts ?? []) {
    mark(
      {
        id: a.id,
        kind: 'alert',
        severity: a.severity ?? 'warning',
        title: `${a.rule_name} ${src.t(`event.${a.event}`)}`,
        detail: a.detail ?? '',
        at: a.at,
        href: a.app ? `/apps/${a.app}/alerts` : '/alerts',
        resolved: a.resolved,
        groupKey: [
          a.rule_id,
          a.app ?? '',
          a.node ?? '',
          a.resource_id ?? '',
          a.event,
        ].join('|'),
      },
      a.read,
    )
  }
  for (const a of src.approvals ?? []) {
    mark({
      id: `approval:${a.id}`,
      kind: 'approval',
      severity: 'warning',
      title: src.t('page.entry.approvalTitle', { app: a.service_name }),
      detail: src.t('page.entry.approvalDetail', {
        action: a.action,
        image: a.image,
        who: a.requested_by_name || a.requested_by,
      }),
      at: a.created_at,
      href: '/approvals',
      action: { type: 'deployApproval', approvalId: a.id },
    })
  }
  for (const s of src.signIns ?? []) {
    mark({
      id: `signin:${s.id}`,
      kind: 'approval',
      severity: 'warning',
      title: src.t('page.entry.signInTitle', { ip: s.requester_ip }),
      detail: s.user_agent,
      at: s.created_at,
      href: '/security',
      action: { type: 'signIn', approvalId: s.id },
    })
  }
  for (const d of src.failedDeploys ?? []) {
    mark({
      id: `deploy:${d.id}`,
      kind: 'deploy',
      severity: 'critical',
      title: src.t('page.entry.deployTitle', { app: d.service_name }),
      detail: d.error ?? '',
      at: d.finished_at ?? d.started_at,
      href: `/apps/${d.service_name}/deploys`,
      action: d.image
        ? { type: 'retryDeploy', app: d.service_name, image: d.image }
        : { type: 'link', to: `/apps/${d.service_name}/deploys` },
    })
  }
  for (const f of src.feed ?? []) {
    const kind = FEED_KINDS[f.kind]
    if (!kind) continue
    mark({
      id: f.id,
      kind,
      severity: f.severity,
      title: f.title,
      detail: f.detail,
      href: f.link,
      action: { type: 'link', to: f.link },
    })
  }
  return out.sort((a, b) => (b.at ?? '').localeCompare(a.at ?? ''))
}

export interface InboxFilters {
  unreadOnly: boolean
  kind: InboxKind | 'all'
  archived: boolean
}

export function filterInboxEntries(
  entries: InboxEntry[],
  f: InboxFilters,
): InboxEntry[] {
  return entries.filter(
    (e) =>
      e.archived === f.archived &&
      (!f.unreadOnly || !e.read) &&
      (f.kind === 'all' || e.kind === f.kind),
  )
}

export function inboxUnread(entries: InboxEntry[]): number {
  return entries.filter((e) => !e.read && !e.archived).length
}
