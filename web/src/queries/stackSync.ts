// Git sync of a stack: GET/PUT /api/v1/stacks/{name}/sync, POST .../sync/refresh
// and POST .../sync/webhook-secret/rotate (internal/api/stacks_sync.go).

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { stackKeys } from './stacks'

export interface StackSyncConfig {
  repo_url: string
  branch: string
  path: string
  credential_ref?: string
  interval_seconds?: number
  webhook_only: boolean
  enabled: boolean
  apply: boolean
  prune: boolean
  deploy: boolean
}

export interface StackSyncStatus {
  commit?: string
  checked_at?: string
  plan_hash?: string
  counts?: Record<string, number>
  pending: boolean
  code?: string
  detail?: string
  applied_commit?: string
  revision?: number
}

export interface StackSyncInfo {
  config: StackSyncConfig | null
  webhook_secret_set: boolean
  status?: StackSyncStatus
}

export interface StackSyncSecret {
  secret: string
  sync_url: string
  refresh_url: string
}

export const stackSyncKeys = {
  detail: (name: string) => [...stackKeys.all, 'sync', name] as const,
}

const base = (name: string) => `/api/v1/stacks/${encodeURIComponent(name)}/sync`

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
    )
  }
  return (await res.json().catch(() => ({}))) as T
}

export function stackSyncQueryOptions(name: string) {
  return queryOptions({
    queryKey: stackSyncKeys.detail(name),
    queryFn: () =>
      request<StackSyncInfo>(base(name), undefined, 'load stack sync'),
    retry: false,
  })
}

// apply, prune and deploy are always sent as booleans: the server refuses a body without them.
export function usePutStackSync(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (cfg: StackSyncConfig) =>
      request<StackSyncInfo>(
        base(name),
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            ...cfg,
            apply: cfg.apply,
            prune: cfg.prune,
            deploy: cfg.deploy,
          }),
        },
        'save stack sync',
      ),
    onSuccess: (info) => {
      qc.setQueryData(stackSyncKeys.detail(name), info)
    },
  })
}

export function useRefreshStackSync(name: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () =>
      request<StackSyncStatus>(
        `${base(name)}/refresh`,
        { method: 'POST' },
        'refresh stack sync',
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: stackSyncKeys.detail(name) }),
  })
}

// gcTime 0: the one-time secret must not linger in the mutation cache.
export function useRotateStackSyncSecret(name: string) {
  const qc = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: () =>
      request<StackSyncSecret>(
        `${base(name)}/webhook-secret/rotate`,
        { method: 'POST' },
        'rotate webhook secret',
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: stackSyncKeys.detail(name) }),
  })
}
