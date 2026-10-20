// Role assignment and environment grants (internal/api/roles_handlers.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { userKeys } from './users'
import type { UserResource } from './users'

export interface EnvironmentChoice {
  id: string
  name: string
  kind?: string
}

export const userAccessKeys = {
  grants: (userId: string) => ['users', 'grants', userId] as const,
  environments: () => ['access', 'environments'] as const,
}

async function failed(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function fetchGrants(userId: string): Promise<string[]> {
  const res = await fetch(
    `/api/v1/users/${encodeURIComponent(userId)}/environment-grants`,
  )
  if (!res.ok) {
    throw await failed(res, 'fetch environment grants')
  }
  return ((await res.json()) as { environment_ids: string[] }).environment_ids
}

export function grantsQueryOptions(userId: string) {
  return queryOptions({
    queryKey: userAccessKeys.grants(userId),
    queryFn: () => fetchGrants(userId),
  })
}

export async function fetchEnvironmentChoices(): Promise<EnvironmentChoice[]> {
  const res = await fetch('/api/v1/environments')
  if (!res.ok) {
    throw await failed(res, 'fetch environments')
  }
  return (await res.json()) as EnvironmentChoice[]
}

export function useEnvironmentChoices(enabled: boolean) {
  return useQuery({
    queryKey: userAccessKeys.environments(),
    queryFn: fetchEnvironmentChoices,
    enabled,
  })
}

export interface AssignRoleRequest {
  userId: string
  roleId: string
  environmentIds?: string[]
}

// Grants are written after the role so a role change that fails never
// leaves grants saved for a user who still holds the old role.
export async function assignRole(
  req: AssignRoleRequest,
): Promise<UserResource> {
  const base = `/api/v1/users/${encodeURIComponent(req.userId)}`
  const roleRes = await fetch(`${base}/role`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ role_id: req.roleId }),
  })
  if (!roleRes.ok) {
    throw await failed(roleRes, 'set role')
  }
  const user = (await roleRes.json()) as UserResource
  if (req.environmentIds) {
    const grantsRes = await fetch(`${base}/environment-grants`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ environment_ids: req.environmentIds }),
    })
    if (!grantsRes.ok) {
      throw await failed(grantsRes, 'set environment grants')
    }
  }
  return user
}

export function useAssignRole() {
  const queryClient = useQueryClient()
  return useMutation<UserResource, ApiError, AssignRoleRequest>({
    mutationFn: assignRole,
    onSuccess: (_user, req) => {
      void queryClient.invalidateQueries({ queryKey: userKeys.list() })
      void queryClient.invalidateQueries({ queryKey: ['roles'] })
      void queryClient.invalidateQueries({
        queryKey: userAccessKeys.grants(req.userId),
      })
    },
  })
}
