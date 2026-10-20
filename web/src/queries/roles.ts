// Fetchers and hooks for internal/api/roles.go: the curated role presets
// a user's abilities can be set to in one action.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { Ability } from '../types/token'

export type RoleVisibility = 'all' | 'granted'

export interface RoleResource {
  name: string
  description: string
  abilities: Ability[]
  id?: string
  visibility?: RoleVisibility
  builtin?: boolean
  user_count?: number
}

export interface RoleInput {
  name: string
  description: string
  abilities: Ability[]
  visibility: RoleVisibility
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

async function sendRole(
  url: string,
  method: 'POST' | 'PUT' | 'DELETE',
  body?: RoleInput,
): Promise<RoleResource | undefined> {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} roles failed: ${res.status}`),
    )
  }
  return method === 'DELETE' ? undefined : ((await res.json()) as RoleResource)
}

export async function createRole(input: RoleInput): Promise<RoleResource> {
  return (await sendRole('/api/v1/roles', 'POST', input)) as RoleResource
}

export async function updateRole(
  id: string,
  input: RoleInput,
): Promise<RoleResource> {
  return (await sendRole(
    `/api/v1/roles/${encodeURIComponent(id)}`,
    'PUT',
    input,
  )) as RoleResource
}

export async function deleteRole(id: string): Promise<void> {
  await sendRole(`/api/v1/roles/${encodeURIComponent(id)}`, 'DELETE')
}

function useRoleMutation<TVars, TData>(fn: (vars: TVars) => Promise<TData>) {
  const queryClient = useQueryClient()
  return useMutation<TData, ApiError, TVars>({
    mutationFn: fn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: roleKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['users'] })
    },
  })
}

export function useCreateRole() {
  return useRoleMutation(createRole)
}

export function useUpdateRole() {
  return useRoleMutation((v: { id: string; input: RoleInput }) =>
    updateRole(v.id, v.input),
  )
}

export function useDeleteRole() {
  return useRoleMutation(deleteRole)
}
