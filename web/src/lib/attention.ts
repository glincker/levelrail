import type { AppListEntry } from '../types/appDetail'
import type { NodeResource } from '../types/nodeDetail'
import type { CertificateStatus } from '../queries/certificates'
import type { DoctorCheck, DoctorReport } from '../queries/systemDoctor'
import type { IssueSilence } from '../queries/silences'
import type { DiskPressure } from './diskPressure'
import type { FailedDeploy } from '../queries/failedDeploys'
import type { UpdateStatus } from '../queries/updates'
import { certExpiryLabel } from './certStatus'
import { formatAge, formatBytes } from './format'
import { nodeAgentAttentionItems } from './nodeAgentAttention'

// [current key, legacy key]. The Status page once saved silences under the
// check code, so both stay honoured while new ones use the name.
export function doctorSilenceKeys(
  check: Pick<DoctorCheck, 'name' | 'code'>,
): [string, string] {
  return [`doctor:${check.name}`, `doctor:${check.code}`]
}

// Same predicate shape, but an id also counts as silenced when one of its
// item's legacy ids is. Every list and count must go through this.
export function silencedWithLegacy(
  items: { id: string; legacyIds?: string[] }[],
  isSnoozed: (id: string) => boolean,
): (id: string) => boolean {
  const legacy = new Map(items.map((i) => [i.id, i.legacyIds ?? []]))
  return (id) =>
    isSnoozed(id) || (legacy.get(id) ?? []).some((l) => isSnoozed(l))
}

// Rewrites a legacy-keyed silence to its item's current id so lists that
// match by issue_key show it once.
export function canonicalSilences(
  items: { id: string; legacyIds?: string[] }[],
  silences: IssueSilence[],
): IssueSilence[] {
  const toCurrent = new Map<string, string>()
  for (const i of items)
    for (const l of i.legacyIds ?? []) toCurrent.set(l, i.id)
  return silences.map((s) => {
    const key = toCurrent.get(s.issue_key)
    return key ? { ...s, issue_key: key } : s
  })
}

export type AttentionSeverity = 'critical' | 'warning' | 'info'

// An incident is broken or about to break and needs action; a
// recommendation is setup or hardening advice. Only incidents reach the
// dashboard and the header chip.
export type AttentionClass = 'incident' | 'recommendation'

export type AttentionTarget =
  | { kind: 'app'; name: string }
  | { kind: 'deploy'; app: string; image: string; lastGoodImage?: string }
  | { kind: 'node'; id: string }
  | { kind: 'domain' }
  | { kind: 'system' }
  | { kind: 'route'; to: '/settings/cli-access' | '/approvals' }
  | { kind: 'path'; to: string }

// What the Status page and the top banner need to label a CLI login and to
// let the operator dismiss it once it is no longer waiting.
export interface AttentionDeviceMeta {
  state: 'waiting' | 'expired' | 'denied' | 'approved'
  deviceName: string
  requesterIp: string
  at: string
  dismissKey?: string
  auditPath?: string
}

export interface AttentionItem {
  id: string
  severity: AttentionSeverity
  title: string
  detail: string
  target: AttentionTarget
  device?: AttentionDeviceMeta
  // Plain next step from the server feed, shown beside the link.
  action?: string
  // Missing means incident, the safe default for a new source.
  class?: AttentionClass
  // Older silence ids that still count for this item (saved before a key change).
  legacyIds?: string[]
  copy?: AttentionCopyMeta
}

// A data copy item: the failure in plain words, the raw error on request,
// and the key a dismissal is remembered under (one copy attempt).
export interface AttentionCopyMeta {
  state: 'failed' | 'stalled'
  hint?: string
  technical?: string
  // Server failure class (ssl, auth, dns...), the root cause a group shares.
  code?: string
  // The managed database the copy targets, for an in-place retry.
  database?: string
  dismissKey: string
}

export function attentionClassOf(item: AttentionItem): AttentionClass {
  return item.class ?? 'incident'
}

export function buildAttentionItems({
  apps = [],
  nodes = [],
  certs = [],
  doctor,
  disk,
  failedDeploys = [],
  updates,
}: {
  apps?: AppListEntry[]
  nodes?: NodeResource[]
  certs?: CertificateStatus[]
  doctor?: DoctorReport
  disk?: DiskPressure
  failedDeploys?: FailedDeploy[]
  updates?: UpdateStatus
}): AttentionItem[] {
  const items: AttentionItem[] = []

  if (disk && disk.level !== 'ok') {
    items.push({
      id: 'disk',
      severity: disk.level === 'critical' ? 'critical' : 'warning',
      title:
        disk.level === 'critical' ? 'Disk almost full' : 'Disk space is low',
      detail: `${formatBytes(disk.freeBytes)} free (${disk.freePercent.toFixed(1)}%), ${formatBytes(disk.reclaimableBytes)} reclaimable`,
      target: { kind: 'system' },
    })
  }

  for (const app of apps) {
    if (app.status.variant === 'destructive') {
      items.push({
        id: `app:${app.name}`,
        severity: 'critical',
        title: app.name,
        detail:
          app.status.label === 'Attention needed'
            ? 'A health or deploy condition is failing. Open the app to see which one.'
            : `Status: ${app.status.label}`,
        target: { kind: 'app', name: app.name },
      })
    }
  }

  for (const d of failedDeploys) {
    items.push({
      id: `deploy:${d.service_name}`,
      severity: 'critical',
      title: `Deploy of ${d.service_name} failed`,
      detail: `${d.error || 'No error recorded'} (${formatAge(d.finished_at ?? d.started_at)})`,
      target: {
        kind: 'deploy',
        app: d.service_name,
        image: d.image,
        lastGoodImage: d.last_good_image,
      },
    })
  }

  for (const node of nodes) {
    if (node.status === 'offline') {
      items.push({
        id: `node:${node.id}`,
        severity: 'critical',
        title: `Node ${node.name} is offline`,
        detail: node.last_seen_at
          ? `Last seen ${formatAge(node.last_seen_at)}`
          : 'Never reported in',
        target: { kind: 'node', id: node.id },
      })
    }
    items.push(...nodeAgentAttentionItems(node))
  }

  for (const cert of certs) {
    if (cert.status === 'healthy' || cert.expiry_class !== 'incident') {
      continue
    }
    const expired = cert.status === 'expired'
    items.push({
      id: `cert:${cert.domain}`,
      severity: expired ? 'critical' : 'warning',
      title: `Certificate for ${cert.domain} ${expired ? 'has expired' : 'expires soon'}`,
      detail: cert.acme_failure?.reason
        ? `${certExpiryLabel(cert.not_after)}. Last CA error: ${cert.acme_failure.reason}`
        : certExpiryLabel(cert.not_after),
      target: { kind: 'domain' },
    })
  }

  for (const check of doctor?.checks ?? []) {
    if (check.status === 'fail' || check.status === 'warn') {
      items.push({
        // Same id the Doctor page and the server silence count use.
        id: doctorSilenceKeys(check)[0],
        legacyIds: doctorSilenceKeys(check).slice(1),
        severity: check.status === 'fail' ? 'critical' : 'warning',
        title: check.name,
        detail: check.message,
        target: { kind: 'system' },
        class:
          check.class ??
          (check.status === 'fail' ? 'incident' : 'recommendation'),
      })
    }
  }

  if (updates?.update_available) {
    items.push({
      id: 'update',
      severity: 'warning',
      title: 'A new version is available',
      detail: updates.latest_version
        ? `${updates.latest_version} (running ${updates.current_version})`
        : `running ${updates.current_version}`,
      target: { kind: 'system' },
      class: 'recommendation',
    })
  }

  return items.sort(
    (a, b) =>
      Number(b.severity === 'critical') - Number(a.severity === 'critical'),
  )
}
