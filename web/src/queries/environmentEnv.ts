// Query-key factory and fetchers for an environment's shared env vars
// (internal/api/environment_env.go): full-replace GET/PUT, the same wire
// shape queries/organizationEnv.ts already establishes one tier up.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { environmentKeys } from './environments'

export function environmentEnvQueryKey(id: string) {
  return [...environmentKeys.detail(id), 'env'] as const
}

export async function fetchEnvironmentEnv(
  id: string,
): Promise<Record<string, string>> {
  const res = await fetch(`/api/v1/environments/${encodeURIComponent(id)}/env`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch environment env failed: ${res.status}`),
    )
  }
  return (await res.json()) as Record<string, string>
}

export function environmentEnvQueryOptions(id: string) {
  return queryOptions({
    queryKey: environmentEnvQueryKey(id),
    queryFn: () => fetchEnvironmentEnv(id),
  })
}

export function useEnvironmentEnv(id: string) {
  return useSuspenseQuery(environmentEnvQueryOptions(id))
}

export async function setEnvironmentEnv(
  id: string,
  vars: Record<string, string>,
): Promise<Record<string, string>> {
  const res = await fetch(`/api/v1/environments/${encodeURIComponent(id)}/env`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(vars),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set environment env failed: ${res.status}`),
    )
  }
  return (await res.json()) as Record<string, string>
}

export function useSetEnvironmentEnv(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (vars: Record<string, string>) => setEnvironmentEnv(id, vars),
    onSuccess: (updated) => {
      queryClient.setQueryData(environmentEnvQueryKey(id), updated)
    },
  })
}
