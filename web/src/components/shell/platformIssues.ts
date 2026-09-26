import type { CertificateStatus } from '../../queries/certificates'
import type { DoctorReport } from '../../queries/systemDoctor'
import type { SystemStatus } from '../../queries/systemStatus'
import type { NodeResource } from '../../types/nodeDetail'
import { assessDiskPressure } from '../../lib/diskPressure'
import { formatBytes } from '../../lib/format'

export type IssueSeverity = 'critical' | 'warning'

export type IssueAction =
  | { kind: 'cleanup-docker' }
  | { kind: 'link'; label: string; to: string; hash?: string }

export interface PlatformIssue {
  id: string
  severity: IssueSeverity
  title: string
  detail: string
  actions: IssueAction[]
}

export interface IssueSources {
  status?: SystemStatus
  nodes?: NodeResource[]
  certs?: CertificateStatus[]
  doctor?: DoctorReport
  insecure?: boolean
  dashboardUrlAnchor?: string
}

export function derivePlatformIssues(src: IssueSources): PlatformIssue[] {
  const issues: PlatformIssue[] = []
  const { status } = src

  if (status && !status.docker_connected) {
    issues.push({
      id: 'docker-down',
      severity: 'critical',
      title: 'Docker is unreachable',
      detail: status.docker_error ?? 'Deploys and restarts will fail',
      actions: [{ kind: 'link', label: 'View node', to: '/nodes' }],
    })
  }

  const disk = status ? assessDiskPressure(status) : undefined
  if (disk && disk.level !== 'ok') {
    issues.push({
      id: 'disk',
      severity: disk.level === 'critical' ? 'critical' : 'warning',
      title:
        disk.level === 'critical' ? 'Disk almost full' : 'Disk space is low',
      detail: `${formatBytes(disk.freeBytes)} free (${disk.freePercent.toFixed(0)}%)`,
      actions: [{ kind: 'cleanup-docker' }],
    })
  }

  for (const node of src.nodes ?? []) {
    if (node.status === 'offline') {
      issues.push({
        id: `node-offline:${node.id}`,
        severity: 'critical',
        title: `${node.name} is offline`,
        detail: 'Apps placed there are not running',
        actions: [
          { kind: 'link', label: 'View node', to: `/nodes/${node.id}` },
        ],
      })
    }
  }

  for (const cert of src.certs ?? []) {
    if (cert.status === 'healthy') continue
    const expired = cert.status === 'expired'
    issues.push({
      id: `cert:${cert.domain}`,
      severity: expired ? 'critical' : 'warning',
      title: `${cert.domain} certificate ${expired ? 'expired' : 'expires soon'}`,
      detail:
        cert.renewal === 'stalled' ? 'Renewal is stalled' : 'Renewal pending',
      actions: [{ kind: 'link', label: 'Renew certificate', to: '/domains' }],
    })
  }

  for (const check of src.doctor?.checks ?? []) {
    if (check.status !== 'fail' && check.status !== 'warn') continue
    if (
      check.code.toLowerCase().includes('disk') &&
      disk &&
      disk.level !== 'ok'
    ) {
      continue
    }
    issues.push({
      id: `doctor:${check.code}`,
      severity: check.status === 'fail' ? 'critical' : 'warning',
      title: check.name,
      detail: check.message,
      actions: [{ kind: 'link', label: 'Open status page', to: '/status' }],
    })
  }

  if (src.insecure) {
    issues.push({
      id: 'insecure-connection',
      severity: 'warning',
      title: 'Dashboard is not on HTTPS',
      detail: 'Sessions travel unencrypted',
      actions: [
        {
          kind: 'link',
          label: 'Set up HTTPS',
          to: '/domains',
          hash: src.dashboardUrlAnchor,
        },
      ],
    })
  }

  return issues.sort(
    (a, b) =>
      Number(b.severity === 'critical') - Number(a.severity === 'critical'),
  )
}

export type ChipLevel = 'ok' | 'warning' | 'critical'

export interface ChipSummary {
  level: ChipLevel
  visible: PlatformIssue[]
  snoozed: PlatformIssue[]
}

export function summarizeIssues(
  issues: PlatformIssue[],
  isSnoozed: (id: string) => boolean,
): ChipSummary {
  const visible = issues.filter((i) => !isSnoozed(i.id))
  const snoozed = issues.filter((i) => isSnoozed(i.id))
  let level: ChipLevel = 'ok'
  if (visible.some((i) => i.severity === 'critical')) level = 'critical'
  else if (visible.length > 0) level = 'warning'
  return { level, visible, snoozed }
}

export type BannerKind = 'connection' | 'docker' | 'disk'

// At most one slim banner, only for problems that block work.
export function pickBanner(input: {
  offline: boolean
  connectionDismissed: boolean
  issues: PlatformIssue[]
  isSnoozed: (id: string) => boolean
}): BannerKind | null {
  if (input.offline && !input.connectionDismissed) return 'connection'
  const live = (id: string) =>
    input.issues.some((i) => i.id === id && i.severity === 'critical') &&
    !input.isSnoozed(`banner:${id}`)
  if (live('docker-down')) return 'docker'
  if (live('disk')) return 'disk'
  return null
}
