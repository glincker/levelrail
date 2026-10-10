// Maps a doctor check to the plain-language guidance and primary action shown on its warning card.

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
  | 'generic'

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
  /** Port number for reachability guidance, interpolated into its copy. */
  port?: string
}

const DOCS_TROUBLESHOOTING = '/troubleshooting'
const SECRET_CODES = ['master_key_rotation', 'secret_binding', 'stale_secrets']

/** guidanceFor picks the guidance entry and action for a check; unmatched checks get generic guidance pointing at system status. */
export function guidanceFor(check: DoctorCheck): Guidance {
  const { code } = check
  const reach = /^external_reachability_(\d+)$/.exec(code)
  if (reach?.[1]) {
    return {
      key: 'reachability',
      action: { kind: 'explainer' },
      port: reach[1],
    }
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
  return {
    key: 'generic',
    action: { kind: 'link', to: '/settings/system-status' },
  }
}
