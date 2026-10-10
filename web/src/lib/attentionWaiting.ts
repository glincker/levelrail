import type { TFunction } from 'i18next'
import type { CertificateStatus } from '../queries/certificates'
import type {
  DeviceActivityItem,
  DeviceAuthRequest,
} from '../queries/deviceAuth'
import type { AttentionFeedItem } from '../queries/attentionFeed'
import type { DeployApprovalResource } from '../types/deployApproval'
import type { NodeResource } from '../types/nodeDetail'
import type { AttentionItem } from './attention'
import {
  formatCountdown,
  liveDeviceRequests,
  msUntilExpiry,
  requesterLabel,
} from './deviceLogin'

type AttentionT = TFunction<'attention'>

// Items that wait on a person or an operator action and were previously
// only visible if you went looking: CLI logins, deploy approvals, stalled
// certificate renewals and nodes that joined but never connected.
export function buildWaitingItems({
  deviceLogins,
  resolvedLogins = [],
  approvals = [],
  certs = [],
  nodes = [],
  feed = [],
  now,
  t,
}: {
  deviceLogins?: DeviceAuthRequest[]
  resolvedLogins?: DeviceActivityItem[]
  approvals?: DeployApprovalResource[]
  certs?: CertificateStatus[]
  nodes?: NodeResource[]
  feed?: AttentionFeedItem[]
  now: number
  t: AttentionT
}): AttentionItem[] {
  const items: AttentionItem[] = []

  for (const r of liveDeviceRequests(deviceLogins, now)) {
    items.push({
      id: `device:${r.user_code}`,
      severity: 'warning',
      title: t('items.deviceLoginTitle', {
        name: requesterLabel(r) || t('deviceLogin.unknownDevice'),
      }),
      detail: t('items.deviceLoginDetail', {
        time: formatCountdown(msUntilExpiry(r, now)),
      }),
      target: { kind: 'route', to: '/settings/cli-access' },
      device: {
        state: 'waiting',
        deviceName: requesterLabel(r),
        requesterIp: r.requester_ip,
        at: r.created_at,
      },
    })
  }

  items.push(...resolvedLoginItems(resolvedLogins, t))

  for (const a of approvals) {
    items.push({
      id: `approval:${a.id}`,
      severity: 'warning',
      title: t('items.approvalTitle', { service: a.service_name }),
      detail: t('items.approvalDetail', {
        who: a.requested_by_name || a.requested_by,
      }),
      target: { kind: 'route', to: '/approvals' },
    })
  }

  for (const c of certs) {
    if (c.renewal !== 'stalled' || c.status === 'expired') {
      continue
    }
    const date = c.not_after.slice(0, 10)
    const reason = c.acme_failure?.reason
    items.push({
      id: `cert-renewal:${c.domain}`,
      severity: 'warning',
      title: t('items.certRenewalTitle', { domain: c.domain }),
      detail: reason
        ? t('items.certRenewalFailure', { date, reason })
        : t('items.certRenewalDetail', { date }),
      target: { kind: 'domain' },
    })
  }

  for (const n of nodes) {
    if (n.status_reason === 'enrolled_never_connected') {
      items.push({
        id: `node-enroll:${n.id}`,
        severity: 'warning',
        title: t('items.nodeEnrollTitle', { name: n.name }),
        detail: t('items.nodeEnrollDetail'),
        target: { kind: 'node', id: n.id },
      })
    }
  }

  items.push(...feedItems(feed, t))

  return items
}

// Expired and denied CLI logins stay visible for the server's window as
// informational items until the operator dismisses them. Approved ones
// need no action, so they only appear in the audit log.
function resolvedLoginItems(
  logins: DeviceActivityItem[],
  t: AttentionT,
): AttentionItem[] {
  const out: AttentionItem[] = []
  for (const l of logins) {
    if (l.dismissed || (l.state !== 'expired' && l.state !== 'denied')) {
      continue
    }
    const name = l.client_name || t('deviceLogin.unknownDevice')
    out.push({
      id: `device-resolved:${l.id}:${l.state}`,
      severity: 'info',
      title:
        l.state === 'expired'
          ? t('items.deviceExpiredTitle', { name })
          : t('items.deviceDeniedTitle', { name }),
      detail:
        l.state === 'expired'
          ? t('items.deviceExpiredDetail')
          : t('items.deviceDeniedDetail'),
      target: { kind: 'route', to: '/settings/cli-access' },
      device: {
        state: l.state,
        deviceName: name,
        requesterIp: l.requester_ip,
        at: l.expires_at,
        dismissKey: l.item_key,
        auditPath: l.audit_path,
      },
    })
  }
  return out
}

function shortDate(iso: string | undefined): string {
  return iso ? iso.slice(0, 10) : ''
}

// One i18n pair per feed kind, so a new server source shows its own copy
// and falls back to the server's English line when a key is missing.
function feedItems(feed: AttentionFeedItem[], t: AttentionT): AttentionItem[] {
  return feed.map((f) => {
    const p = f.params ?? {}
    const name = p.name ?? f.subject
    const base = {
      id: f.id,
      severity: f.severity,
      action: f.action,
      target: { kind: 'path', to: f.link } as const,
    }
    switch (f.kind) {
      case 'token_expiring':
        return {
          ...base,
          title: t('items.tokenExpiringTitle', { name }),
          detail: t('items.tokenExpiringDetail', { date: shortDate(p.at) }),
        }
      case 'token_expired':
        return {
          ...base,
          title: t('items.tokenExpiredTitle', { name }),
          detail: t('items.tokenExpiredDetail', { date: shortDate(p.at) }),
        }
      case 'data_copy':
        return {
          ...base,
          title:
            p.state === 'failed'
              ? t('items.dataCopyFailedTitle', { name })
              : t('items.dataCopyStalledTitle', { name }),
          detail: p.reason
            ? t('items.dataCopyReason', { reason: p.reason })
            : f.detail,
        }
      case 'backup_overdue':
        return {
          ...base,
          title: t('items.backupOverdueTitle', { name }),
          detail: p.since
            ? t('items.backupOverdueDetail', { date: shortDate(p.since) })
            : f.detail,
        }
      case 'invites':
        return {
          ...base,
          title: t('items.invitesTitle'),
          detail: t('items.invitesDetail', {
            pending: p.pending ?? '0',
            expired: p.expired ?? '0',
          }),
        }
      case 'login_code':
        return {
          ...base,
          title: t('items.loginCodeTitle', {
            ip: p.ip ?? f.subject,
            count: Number.parseInt(p.count ?? '1', 10) || 1,
          }),
          detail: t('items.loginCodeDetail', { agent: p.user_agent ?? '' }),
        }
      case 'login_approval':
        return {
          ...base,
          title: t('items.loginApprovalTitle', { ip: p.ip ?? f.subject }),
          detail: t('items.loginApprovalDetail', { agent: p.user_agent ?? '' }),
        }
      default:
        return { ...base, title: f.title || f.subject, detail: f.detail }
    }
  })
}

const WAITING_ID_PREFIXES = [
  'device:',
  'approval:',
  'login_code:',
  'login_approval:',
]

// Critical first, then items waiting on a person ahead of the rest. The
// sort is stable, so ties keep their source order.
export function rankAttentionItems(items: AttentionItem[]): AttentionItem[] {
  const waits = (i: AttentionItem) =>
    WAITING_ID_PREFIXES.some((p) => i.id.startsWith(p)) ? 0 : 1
  const rank = (i: AttentionItem) =>
    i.severity === 'critical' ? 0 : i.severity === 'info' ? 2 : 1
  return [...items].sort((a, b) => rank(a) - rank(b) || waits(a) - waits(b))
}
