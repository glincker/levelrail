// Query-key factory and fetchers for an organization's shared env vars
// (internal/api/organization_env.go): full-replace GET/PUT, the same
// wire shape project_env_vars already establishes one tier down
// (internal/store/project_env.go), just with no CLI/UI counterpart yet
// to mirror there.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { organizationKeys } from './organizations'

export function organizationEnvQueryKey(id: string) {
  return [...organizationKeys.detail(id), 'env'] as const
}

export async function fetchOrganizationEnv(
  id: string,
): Promise<Record<string, string>> {
  const res = await fetch(`/api/v1/organizations/${encodeURIComponent(id)}/env`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch organization env failed: ${res.status}`),
    )
  }
  return (await res.json()) as Record<string, string>
}

export function organizationEnvQueryOptions(id: string) {
  return queryOptions({
    queryKey: organizationEnvQueryKey(id),
    queryFn: () => fetchOrganizationEnv(id),
  })
}

export function useOrganizationEnv(id: string) {
  return useSuspenseQuery(organizationEnvQueryOptions(id))
}

export async function setOrganizationEnv(
  id: string,
  vars: Record<string, string>,
): Promise<Record<string, string>> {
  const res = await fetch(`/api/v1/organizations/${encodeURIComponent(id)}/env`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(vars),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set organization env failed: ${res.status}`),
    )
  }
  return (await res.json()) as Record<string, string>
}

export function useSetOrganizationEnv(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (vars: Record<string, string>) => setOrganizationEnv(id, vars),
    onSuccess: (updated) => {
      queryClient.setQueryData(organizationEnvQueryKey(id), updated)
    },
  })
}
