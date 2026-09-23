import type { AppListEntry } from '../types/appDetail'
import type { NodeResource } from '../types/nodeDetail'
import type { CertificateStatus } from '../queries/certificates'
import type { DoctorReport } from '../queries/systemDoctor'

export type AttentionSeverity = 'critical' | 'warning'

export type AttentionTarget =
  | { kind: 'app'; name: string }
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
}: {
  apps?: AppListEntry[]
  nodes?: NodeResource[]
  certs?: CertificateStatus[]
  doctor?: DoctorReport
}): AttentionItem[] {
  const items: AttentionItem[] = []

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

  for (const node of nodes) {
    if (node.status === 'offline') {
      items.push({
        id: `node:${node.id}`,
        severity: 'critical',
        title: `Node ${node.name} is offline`,
        detail: node.last_seen_at
          ? `Last seen ${node.last_seen_at}`
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
      detail: `Valid until ${cert.not_after}`,
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
