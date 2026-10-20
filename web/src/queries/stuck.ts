import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type StuckKind = 'app' | 'database' | 'domain' | 'node'

export type StuckActionType =
  'fix' | 'recheck' | 'logs' | 'retry_deploy' | 'link'

export interface StuckAction {
  type: StuckActionType
  label: string
  fix_id?: string
  href?: string
}

export interface StuckCause {
  code: string
  title: string
  explanation: string
  confidence: 'high' | 'medium'
  evidence: string[]
  actions: StuckAction[]
}

export interface StuckCondition {
  type: string
  status: string
  reason: string
  message?: string
  since: string
}

// Matches internal/stuck's Report.
export interface StuckReport {
  kind: StuckKind
  name: string
  ready: boolean
  stuck: boolean
  not_ready_since?: string
  not_ready_seconds: number
  threshold_seconds: number
  timeline: StuckCondition[]
  causes: StuckCause[]
}

export const stuckKeys = {
  detail: (kind: StuckKind, name: string) => ['stuck', kind, name] as const,
}

export async function fetchStuck(
  kind: StuckKind,
  name: string,
): Promise<StuckReport> {
  const res = await fetch(
    `/api/v1/diagnose/${kind}/${encodeURIComponent(name)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch diagnosis failed: ${res.status}`),
    )
  }
  return (await res.json()) as StuckReport
}

export function stuckQueryOptions(kind: StuckKind, name: string) {
  return queryOptions({
    queryKey: stuckKeys.detail(kind, name),
    queryFn: () => fetchStuck(kind, name),
    refetchInterval: 30_000,
  })
}

export function useStuck(kind: StuckKind, name: string) {
  return useQuery(stuckQueryOptions(kind, name))
}
