import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type QuotaScope = 'team' | 'project'

export type QuotaResourceKey =
  'apps' | 'databases' | 'memory' | 'cpu' | 'previews'

export interface QuotaLimits {
  max_apps: number
  max_databases: number
  max_memory_mb: number
  max_cpu: number
  max_previews: number
}

export interface QuotaUsage {
  apps: number
  databases: number
  memory_mb: number
  cpu: number
  previews: number
}

export interface QuotaResource {
  scope: QuotaScope
  scope_id: string
  name: string
  limits: QuotaLimits
  effective_limits: QuotaLimits
  usage: QuotaUsage
  warn_percent: number
  can_manage: boolean
}

export const quotaKeys = {
  all: ['quotas'] as const,
  one: (scope: QuotaScope, id: string) =>
    [...quotaKeys.all, scope, id] as const,
}

const quotaPath = (scope: QuotaScope, id: string) =>
  `/api/v1/${scope}s/${encodeURIComponent(id)}/quota`

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function quotaQueryOptions(scope: QuotaScope, id: string) {
  return queryOptions({
    queryKey: quotaKeys.one(scope, id),
    queryFn: () => request<QuotaResource>('GET', quotaPath(scope, id)),
    enabled: id !== '',
  })
}

export function useSetQuota(scope: QuotaScope, id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (limits: QuotaLimits) =>
      request<QuotaResource>('PUT', quotaPath(scope, id), limits),
    onSuccess: () => qc.invalidateQueries({ queryKey: quotaKeys.all }),
  })
}

export function useClearQuota(scope: QuotaScope, id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => request<QuotaResource>('DELETE', quotaPath(scope, id)),
    onSuccess: () => qc.invalidateQueries({ queryKey: quotaKeys.all }),
  })
}
