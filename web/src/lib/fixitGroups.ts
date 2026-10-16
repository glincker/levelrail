import type { MeshCheck } from '../queries/meshSettings'

// Checks that cannot pass while their parent is blocked.
const DEPENDS_ON: Record<string, string> = {
  udp_port: 'wireguard',
  firewall: 'wireguard',
}

export interface CheckGroups {
  primary: MeshCheck[]
  dependent: MeshCheck[]
  blockers: number
}

export function groupBlockedChecks(checks: MeshCheck[]): CheckGroups {
  const failed = new Set(
    checks.filter((c) => c.status === 'fail').map((c) => c.id),
  )
  const primary: MeshCheck[] = []
  const dependent: MeshCheck[] = []
  for (const c of checks) {
    const parent = DEPENDS_ON[c.id]
    if (parent && failed.has(parent) && c.status !== 'pass') {
      dependent.push(c)
    } else {
      primary.push(c)
    }
  }
  return {
    primary,
    dependent,
    blockers: primary.filter((c) => c.status === 'fail').length,
  }
}
