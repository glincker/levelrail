// Query-key factory and fetchers for a project's shared env vars
// (internal/api/project_env.go): full-replace GET/PUT, the same wire
// shape queries/organizationEnv.ts and queries/environmentEnv.ts already
// establish one tier above and below.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { projectKeys } from './projects'

export function projectEnvQueryKey(id: string) {
  return [...projectKeys.detail(id), 'env'] as const
}

export async function fetchProjectEnv(
  id: string,
): Promise<Record<string, string>> {
  const res = await fetch(`/api/v1/projects/${encodeURIComponent(id)}/env`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch project env failed: ${res.status}`),
    )
  }
  return (await res.json()) as Record<string, string>
}

export function projectEnvQueryOptions(id: string) {
  return queryOptions({
    queryKey: projectEnvQueryKey(id),
    queryFn: () => fetchProjectEnv(id),
  })
}

export function useProjectEnv(id: string) {
  return useSuspenseQuery(projectEnvQueryOptions(id))
}

export async function setProjectEnv(
  id: string,
  vars: Record<string, string>,
): Promise<Record<string, string>> {
  const res = await fetch(`/api/v1/projects/${encodeURIComponent(id)}/env`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(vars),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set project env failed: ${res.status}`),
    )
  }
  return (await res.json()) as Record<string, string>
}

export function useSetProjectEnv(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (vars: Record<string, string>) => setProjectEnv(id, vars),
    onSuccess: (updated) => {
      queryClient.setQueryData(projectEnvQueryKey(id), updated)
    },
  })
}
