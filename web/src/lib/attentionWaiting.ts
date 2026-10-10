import type { TFunction } from 'i18next'
import type { CertificateStatus } from '../queries/certificates'
import type { DeviceAuthRequest } from '../queries/deviceAuth'
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
  approvals = [],
  certs = [],
  nodes = [],
  now,
  t,
}: {
  deviceLogins?: DeviceAuthRequest[]
  approvals?: DeployApprovalResource[]
  certs?: CertificateStatus[]
  nodes?: NodeResource[]
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
    })
  }

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

  return items
}

const WAITING_ID_PREFIXES = ['device:', 'approval:']

// Critical first, then items waiting on a person ahead of the rest. The
// sort is stable, so ties keep their source order.
export function rankAttentionItems(items: AttentionItem[]): AttentionItem[] {
  const waits = (i: AttentionItem) =>
    WAITING_ID_PREFIXES.some((p) => i.id.startsWith(p)) ? 0 : 1
  return [...items].sort(
    (a, b) =>
      Number(b.severity === 'critical') - Number(a.severity === 'critical') ||
      waits(a) - waits(b),
  )
}
