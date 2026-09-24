import type { AppListEntry } from '../types/appDetail'
import type { NodeResource } from '../types/nodeDetail'
import type { CertificateStatus } from '../queries/certificates'
import type { DoctorReport } from '../queries/systemDoctor'
import type { DiskPressure } from './diskPressure'
import type { FailedDeploy } from '../queries/failedDeploys'
import { certExpiryLabel } from './certStatus'
import { formatAge, formatBytes } from './format'

export type AttentionSeverity = 'critical' | 'warning'

export type AttentionTarget =
  | { kind: 'app'; name: string }
  | { kind: 'deploy'; app: string; image: string; lastGoodImage?: string }
  | { kind: 'node'; id: string }
  | { kind: 'domain' }
  | { kind: 'system' }

export interface AttentionItem {
  id: string
  severity: AttentionSeverity
  title: string
  detail: string
  target: AttentionTarget
}

export function buildAttentionItems({
  apps = [],
  nodes = [],
  certs = [],
  doctor,
  disk,
  failedDeploys = [],
}: {
  apps?: AppListEntry[]
  nodes?: NodeResource[]
  certs?: CertificateStatus[]
  doctor?: DoctorReport
  disk?: DiskPressure
  failedDeploys?: FailedDeploy[]
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
        detail: app.status.label,
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
  }

  for (const cert of certs) {
    if (cert.status === 'healthy') {
      continue
    }
    const expired = cert.status === 'expired'
    items.push({
      id: `cert:${cert.domain}`,
      severity: expired ? 'critical' : 'warning',
      title: `Certificate for ${cert.domain} ${expired ? 'has expired' : 'expires soon'}`,
      detail: certExpiryLabel(cert.not_after),
      target: { kind: 'domain' },
    })
  }

  for (const check of doctor?.checks ?? []) {
    if (check.status === 'fail' || check.status === 'warn') {
      items.push({
        id: `doctor:${check.code}`,
        severity: check.status === 'fail' ? 'critical' : 'warning',
        title: check.name,
        detail: check.message,
        target: { kind: 'system' },
      })
    }
  }

  return items.sort(
    (a, b) =>
      Number(b.severity === 'critical') - Number(a.severity === 'critical'),
  )
}
