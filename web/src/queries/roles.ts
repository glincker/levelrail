// Fetchers and hooks for internal/api/roles.go: the curated role presets
// a user's abilities can be set to in one action.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { Ability } from '../types/token'

export interface RoleResource {
  name: string
  description: string
  abilities: Ability[]
}

export const roleKeys = {
  all: ['roles'] as const,
  list: () => [...roleKeys.all, 'list'] as const,
}

export async function fetchRoles(): Promise<RoleResource[]> {
  const res = await fetch('/api/v1/roles')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch roles failed: ${res.status}`),
    )
  }
  return (await res.json()) as RoleResource[]
}

export function roleListQueryOptions() {
  return queryOptions({ queryKey: roleKeys.list(), queryFn: fetchRoles })
}

export function useRoles() {
  return useSuspenseQuery(roleListQueryOptions())
}

// abilitySetsEqual is order-insensitive: mirrors internal/api/roles.go's
// own abilitySetsEqual so a hand-picked ability list that happens to
// match a preset (in any order) is still recognized as that role, not
// shown as "Custom".
export function abilitySetsEqual(a: Ability[], b: Ability[]): boolean {
  if (a.length !== b.length) {
    return false
  }
  const setB = new Set(b)
  return a.every((ability) => setB.has(ability))
}

// roleForAbilities mirrors internal/api/roles.go's roleForAbilities: the
// curated role whose ability set exactly matches abilities, or undefined
// for the "Custom" case.
export function roleForAbilities(
  roles: RoleResource[],
  abilities: Ability[],
): RoleResource | undefined {
  return roles.find((role) => abilitySetsEqual(role.abilities, abilities))
}
