import { useQuery } from '@tanstack/react-query'
import { userListQueryOptions } from '../queries/users'
import { hasAbility, type Ability } from '../types/token'
import { useAuthUsername } from './useAuthUsername'

export type AbilityRole = 'admin' | 'operator' | 'viewer' | 'custom'

export interface Abilities {
  /** Curated role of the signed-in user, or null while it is unknown. */
  role: AbilityRole | null
  can: (ability: Ability) => boolean
}

// Mirrors the curated presets in internal/api/roles.go.
function roleOf(granted: Ability[]): AbilityRole {
  if (granted.includes('root')) return 'admin'
  if (granted.includes('write') || granted.includes('deploy')) return 'operator'
  if (granted.length === 1 && granted[0] === 'read') return 'viewer'
  return 'custom'
}

// UX-only gate like useIsRoot: the server enforces every ability. While the
// user list loads, nothing is allowed so a gated control never flashes on.
export function useAbilities(): Abilities {
  const ownEmail = useAuthUsername()
  const { data: users } = useQuery(userListQueryOptions())
  const own = Array.isArray(users)
    ? users.find((u) => u.email === ownEmail)
    : undefined
  if (!own) return { role: null, can: () => false }
  return {
    role: roleOf(own.abilities),
    can: (ability) => hasAbility(own.abilities, ability),
  }
}
