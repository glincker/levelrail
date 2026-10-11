// Guided cutover of one staged app (internal/api/app_import_cutover_run.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type CutoverStatus = 'pass' | 'warn' | 'block'

export interface CutoverFix {
  summary: string
  action?: { kind: 'request' | 'link' | 'copy'; label: string; value?: string }
}

export interface CutoverCheck {
  id: string
  title: string
  status: CutoverStatus
  detail?: string
  fix?: CutoverFix
}

export interface CutoverRecord {
  name: string
  type: string
  value: string
  ttl_seconds?: number
}

export interface CutoverDomainPlan {
  domain: string
  method: 'dns' | 'proxy' | 'manual' | 'none'
  current?: string[]
  provider?: string
  desired?: CutoverRecord
  message?: string
}

export interface CutoverPlan {
  app: string
  verdict: 'ready' | 'warnings' | 'blocked'
  checks: CutoverCheck[]
  domains: CutoverDomainPlan[]
  health_path: string
}

export type CutoverState =
  | 'planning'
  | 'ready'
  | 'starting'
  | 'verifying'
  | 'switching'
  | 'live'
  | 'rolled_back'
  | 'failed'

export interface CutoverStep {
  name: string
  state: 'running' | 'done' | 'failed' | 'skipped'
  domain?: string
  detail?: string
  duration_ms: number
}

export interface CutoverDomainRun {
  domain: string
  method: string
  previous?: CutoverRecord[]
  applied?: CutoverRecord
  manual?: CutoverRecord
  switched: boolean
  restored: boolean
  verified: boolean
  verify_detail?: string
}

export interface CutoverRun {
  id: string
  app: string
  mode: 'dry_run' | 'switch'
  state: CutoverState
  method?: string
  awaiting?: string
  domains: CutoverDomainRun[]
  steps: CutoverStep[]
  error?: string
  created_at: string
  updated_at: string
  rollbackable: boolean
  in_flight: boolean
}

export function cutoverSettled(run: CutoverRun | undefined): boolean {
  if (!run) return true
  if (run.state === 'switching')
    return run.awaiting !== undefined && run.awaiting !== ''
  return !['planning', 'starting', 'verifying'].includes(run.state)
}

const base = '/api/v1/migration/apps/sessions'
const enc = encodeURIComponent

export const cutoverKeys = {
  plan: (id: string, item: string) =>
    ['migration', 'apps', 'cutover-plan', id, item] as const,
  runs: (id: string, item: string) =>
    ['migration', 'apps', 'cutover-runs', id, item] as const,
}

function path(id: string, item: string): string {
  return `${base}/${enc(id)}/items/${enc(item)}/cutover`
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

export function useCutoverPlan(id: string, item: string, enabled: boolean) {
  return useQuery({
    queryKey: cutoverKeys.plan(id, item),
    queryFn: () => send<CutoverPlan>(`${path(id, item)}/plan`),
    enabled,
    staleTime: 0,
  })
}

export function useCutoverRuns(id: string, item: string, enabled: boolean) {
  return useQuery({
    queryKey: cutoverKeys.runs(id, item),
    queryFn: async () =>
      (await send<{ runs: CutoverRun[] }>(`${path(id, item)}/runs`)).runs,
    enabled,
    refetchInterval: (q) => (cutoverSettled(q.state.data?.[0]) ? false : 1500),
  })
}

function useInvalidateRuns(id: string, item: string) {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: cutoverKeys.runs(id, item) })
}

export function useStartCutover(id: string, item: string) {
  const invalidate = useInvalidateRuns(id, item)
  return useMutation<
    CutoverRun,
    ApiError,
    { mode: 'dry_run' | 'switch'; confirm?: string; acceptWarnings?: boolean }
  >({
    mutationFn: ({ mode, confirm, acceptWarnings }) =>
      send<CutoverRun>(
        `${path(id, item)}/runs`,
        post({ mode, confirm, accept_warnings: acceptWarnings }),
      ),
    onSuccess: invalidate,
  })
}

export function useRollbackCutover(id: string, item: string) {
  const invalidate = useInvalidateRuns(id, item)
  return useMutation<CutoverRun, ApiError, { run: string }>({
    mutationFn: ({ run }) =>
      send<CutoverRun>(`${path(id, item)}/runs/${enc(run)}/rollback`, post({})),
    onSuccess: invalidate,
  })
}

export function useConfirmCutoverDns(id: string, item: string) {
  const invalidate = useInvalidateRuns(id, item)
  return useMutation<CutoverRun, ApiError, { run: string }>({
    mutationFn: ({ run }) =>
      send<CutoverRun>(
        `${path(id, item)}/runs/${enc(run)}/confirm-dns`,
        post({}),
      ),
    onSuccess: invalidate,
  })
}
