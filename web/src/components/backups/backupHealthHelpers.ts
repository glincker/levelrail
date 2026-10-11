import type {
  BackupHealthState,
  ResourceHealth,
} from '../../types/backupProtection'

export const DATABASES_GROUP = '\u0000databases'

export interface HealthGroup {
  key: string
  app?: string
  resources: ResourceHealth[]
}

// groupHealth puts every volume under its app and every database under one
// shared group, apps first, each group's worst state first.
export function groupHealth(resources: ResourceHealth[]): HealthGroup[] {
  const groups = new Map<string, HealthGroup>()
  for (const r of resources) {
    const key = r.kind === 'volume' ? (r.app_name ?? '') : DATABASES_GROUP
    let g = groups.get(key)
    if (!g) {
      g = {
        key,
        app: r.kind === 'volume' ? r.app_name : undefined,
        resources: [],
      }
      groups.set(key, g)
    }
    g.resources.push(r)
  }
  const order = (a: HealthGroup, b: HealthGroup) => {
    if (a.key === DATABASES_GROUP) return 1
    if (b.key === DATABASES_GROUP) return -1
    return a.key.localeCompare(b.key)
  }
  return [...groups.values()].sort(order)
}

const STATE_RANK: Record<BackupHealthState, number> = {
  failing: 0,
  unprotected: 1,
  warning: 2,
  unverified: 3,
  healthy: 4,
}

export function sortByState(resources: ResourceHealth[]): ResourceHealth[] {
  return [...resources].sort(
    (a, b) =>
      STATE_RANK[a.state] - STATE_RANK[b.state] ||
      a.resource_name.localeCompare(b.resource_name),
  )
}

export function stateBadgeVariant(
  state: BackupHealthState,
): 'success' | 'warning' | 'destructive' | 'muted' {
  switch (state) {
    case 'healthy':
      return 'success'
    case 'warning':
    case 'unverified':
      return 'warning'
    case 'failing':
      return 'destructive'
    default:
      return 'muted'
  }
}
