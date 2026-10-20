// Maps doctor checks to the plain-language guidance and primary action shown on warning cards.

import type { DoctorCheck } from '../queries/systemDoctor'

export type GuidanceKey =
  | 'firewall'
  | 'backup'
  | 'port'
  | 'reachability'
  | 'registry'
  | 'disk'
  | 'diskLatency'
  | 'capacityLow'
  | 'secrets'
  | 'clock'
  | 'publicIp'
  | 'hardening'
  | 'agentAdvertise'
  | 'runtime'

export type GuidancePath =
  | '/settings/registry-credentials'
  | '/settings/firewall'
  | '/settings/backup-targets'
  | '/settings/storage'
  | '/settings/security'
  | '/settings/general'
  | '/settings/system-status'

export type GuidanceAction =
  | { kind: 'link'; to: GuidancePath }
  | { kind: 'docs'; path: string }
  | { kind: 'explainer' }

export interface Guidance {
  key: GuidanceKey
  action: GuidanceAction
}

const DOCS_TROUBLESHOOTING = '/troubleshooting'
const SECRET_CODES = ['master_key_rotation', 'secret_binding', 'stale_secrets']
const REACH_RE = /^external_reachability_(\d+)$/

/** guidanceFor returns written guidance for a check, or null when none exists so the card shows the check's own message instead of filler. */
export function guidanceFor(check: DoctorCheck): Guidance | null {
  const { code } = check
  if (REACH_RE.test(code)) {
    return { key: 'reachability', action: { kind: 'explainer' } }
  }
  if (/^port_\d+$/.test(code)) {
    return { key: 'port', action: { kind: 'docs', path: DOCS_TROUBLESHOOTING } }
  }
  if (code.startsWith('registry_reachability_')) {
    return {
      key: 'registry',
      action: { kind: 'link', to: '/settings/registry-credentials' },
    }
  }
  if (code === 'firewall') {
    return {
      key: 'firewall',
      action: { kind: 'link', to: '/settings/firewall' },
    }
  }
  if (code === 'control_plane_backup' || code === 'control_plane_dr') {
    return {
      key: 'backup',
      action: { kind: 'link', to: '/settings/backup-targets' },
    }
  }
  if (code === 'disk_space') {
    return { key: 'disk', action: { kind: 'link', to: '/settings/storage' } }
  }
  if (code === 'disk_io_latency') {
    return {
      key: 'diskLatency',
      action: { kind: 'docs', path: DOCS_TROUBLESHOOTING },
    }
  }
  if (code === 'ram' || code === 'cpu') {
    return {
      key: 'capacityLow',
      action: { kind: 'docs', path: DOCS_TROUBLESHOOTING },
    }
  }
  if (SECRET_CODES.includes(code)) {
    return {
      key: 'secrets',
      action: { kind: 'link', to: '/settings/security' },
    }
  }
  if (code === 'clock_skew') {
    return {
      key: 'clock',
      action: { kind: 'docs', path: DOCS_TROUBLESHOOTING },
    }
  }
  if (code === 'container_hardening') {
    return {
      key: 'hardening',
      action: { kind: 'docs', path: '/security#fresh-box-hardening-checklist' },
    }
  }
  if (code === 'container_runtime') {
    return {
      key: 'runtime',
      action: { kind: 'docs', path: '/security#rootless-and-podman' },
    }
  }
  if (code === 'agent_advertise_reachability') {
    return {
      key: 'agentAdvertise',
      action: { kind: 'docs', path: DOCS_TROUBLESHOOTING },
    }
  }
  if (code === 'public_ip') {
    return {
      key: 'publicIp',
      action: { kind: 'link', to: '/settings/general' },
    }
  }
  return null
}

export interface WarningGroup {
  id: string
  checks: DoctorCheck[]
  guidance: Guidance | null
  /** Ports covered by a grouped reachability or port-conflict card, in ascending order. */
  ports: string[]
  /** Highest severity among the grouped checks: 0 fail, 1 warn, 2 unknown. */
  severity: number
}

const SEVERITY: Record<DoctorCheck['status'], number> = {
  fail: 0,
  warn: 1,
  unknown: 2,
  ok: 3,
}

function portOf(code: string): string | undefined {
  return /_(\d+)$/.exec(code)?.[1]
}

/** groupWarnings merges related checks into one group (all external reachability ports together, all port conflicts together) and orders groups by severity. */
export function groupWarnings(checks: DoctorCheck[]): WarningGroup[] {
  const groups = new Map<string, WarningGroup>()
  for (const check of checks) {
    const guidance = guidanceFor(check)
    const mergeKey =
      guidance?.key === 'reachability' || guidance?.key === 'port'
        ? guidance.key
        : check.code
    const existing = groups.get(mergeKey)
    const severity = SEVERITY[check.status]
    if (existing) {
      existing.checks.push(check)
      existing.severity = Math.min(existing.severity, severity)
      continue
    }
    groups.set(mergeKey, {
      id: mergeKey,
      checks: [check],
      guidance,
      ports: [],
      severity,
    })
  }
  const out = Array.from(groups.values())
  for (const g of out) {
    g.ports = g.checks
      .map((c) => portOf(c.code))
      .filter(
        (p): p is string =>
          p !== undefined &&
          g.guidance !== null &&
          (g.guidance.key === 'reachability' || g.guidance.key === 'port'),
      )
      .sort((a, b) => Number(a) - Number(b))
  }
  return out.sort((a, b) => a.severity - b.severity)
}

/** joinPorts renders ["80","443"] as "80 and 443" for card copy. */
export function joinPorts(ports: string[]): string {
  if (ports.length <= 1) return ports[0] ?? ''
  return `${ports.slice(0, -1).join(', ')} and ${ports[ports.length - 1] ?? ''}`
}
