// Same-server port takeover (internal/api/takeover.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { beginRestart } from '../lib/restartTracker'
import { fetchSystemRestart } from './systemRestart'

export type TakeoverCheckStatus = 'pass' | 'warn' | 'block'

export interface TakeoverFix {
  summary: string
  action?: {
    kind: 'request' | 'link' | 'copy'
    label: string
    api?: string
    value?: string
  }
}

export interface TakeoverCheck {
  id: string
  title: string
  status: TakeoverCheckStatus
  detail?: string
  items?: string[]
  fix?: TakeoverFix
}

export interface TakeoverPlanStep {
  id: string
  title: string
  detail: string
  reversible: boolean
}

export interface TakeoverDomain {
  domain: string
  app: string
  healthy: boolean
  detail?: string
  dns: 'ok' | 'elsewhere' | 'unresolved'
  certificate: 'custom' | 'acme'
}

export type TakeoverVerdict = 'ready' | 'warnings' | 'blocked' | 'done'

export interface TakeoverPlan {
  verdict: TakeoverVerdict
  checks: TakeoverCheck[]
  steps: TakeoverPlanStep[]
  domains: TakeoverDomain[]
  rollback: string[]
  window_seconds: number
  target_http: string
  target_https: string
  current_http: string
  current_https: string
  proxy_kind?: string
  proxy_containers: string[]
  checked_at: string
}

export type TakeoverState =
  | 'preparing'
  | 'applying'
  | 'verifying'
  | 'done'
  | 'rolling_back'
  | 'rolled_back'
  | 'failed'

export interface TakeoverStep {
  id: string
  title: string
  state: 'pending' | 'running' | 'done' | 'failed' | 'skipped'
  detail?: string
}

export interface TakeoverDomainCheck {
  domain: string
  ok: boolean
  status?: number
  delivered: boolean
  detail?: string
}

export interface TakeoverRun {
  id: string
  state: TakeoverState
  steps: TakeoverStep[]
  verify: TakeoverDomainCheck[]
  error?: string
  window_seconds: number
  deadline_at?: string
  created_at: string
  updated_at: string
  rollbackable: boolean
  in_flight: boolean
}

export interface TakeoverStatus {
  plan: TakeoverPlan
  run: TakeoverRun | null
}

export function takeoverSettled(run: TakeoverRun | null | undefined): boolean {
  return !run || !run.in_flight
}

const base = '/api/v1/migration/takeover'

export const takeoverKeys = {
  status: (acceptUnmanaged: boolean) =>
    ['migration', 'takeover', 'status', acceptUnmanaged] as const,
  runs: ['migration', 'takeover', 'runs'] as const,
}

async function send<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function post(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

// The control plane restarts in the middle of a run, so polling tolerates the gap.
export function useTakeoverStatus(acceptUnmanaged: boolean) {
  return useQuery({
    queryKey: takeoverKeys.status(acceptUnmanaged),
    queryFn: () =>
      send<TakeoverStatus>(
        `${base}/plan${acceptUnmanaged ? '?accept_unmanaged=true' : ''}`,
      ),
    staleTime: 0,
    retry: (count, err) =>
      count < 20 && !(err instanceof ApiError && err.status < 500),
    retryDelay: 1500,
    refetchInterval: (q) => (takeoverSettled(q.state.data?.run) ? false : 2500),
  })
}

export function useStartTakeover() {
  const qc = useQueryClient()
  return useMutation<
    { run?: TakeoverRun; plan?: TakeoverPlan },
    ApiError,
    { confirm: boolean; acceptUnmanaged: boolean }
  >({
    mutationFn: ({ confirm, acceptUnmanaged }) =>
      send(
        `${base}/runs`,
        post({ confirm, accept_unmanaged: acceptUnmanaged }),
      ),
    onSuccess: async (res, vars) => {
      if (vars.confirm && res.run) {
        const info = await fetchSystemRestart().catch(() => null)
        if (info) beginRestart(info.boot_id)
      }
      await qc.invalidateQueries({ queryKey: ['migration', 'takeover'] })
    },
  })
}

export function useRollbackTakeover() {
  const qc = useQueryClient()
  return useMutation<{ run?: TakeoverRun }, ApiError, { run: string }>({
    mutationFn: ({ run }) =>
      send(`${base}/runs/${encodeURIComponent(run)}/rollback`, post({})),
    onSuccess: async () => {
      const info = await fetchSystemRestart().catch(() => null)
      if (info) beginRestart(info.boot_id)
      await qc.invalidateQueries({ queryKey: ['migration', 'takeover'] })
    },
  })
}
