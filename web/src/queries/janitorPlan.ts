// Plan, preview and apply for the cleanup drawer over /api/v1/system/janitor.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { JanitorReport, JanitorStageName } from './janitor'

export interface JanitorItemSelection {
  stage: JanitorStageName
  names: string[]
}

export interface JanitorIgnoredItem {
  stage: JanitorStageName
  name: string
  reason: string
}

export interface JanitorActor {
  name: string
  kind: 'user' | 'api_token' | 'schedule'
}

export interface JanitorPlanReport extends JanitorReport {
  actor?: JanitorActor
  ignored_items?: JanitorIgnoredItem[]
}

export const janitorPlanKeys = {
  plan: ['janitor', 'plan'] as const,
  history: ['janitor', 'runs', 'history'] as const,
}

export interface PlanRunInput {
  dryRun: boolean
  /** Omitted for the first full dry run, which lists every candidate. */
  items?: JanitorItemSelection[]
}

async function runPlan(input: PlanRunInput): Promise<JanitorPlanReport> {
  const res = await fetch(
    `/api/v1/system/janitor/run?dry_run=${input.dryRun ? 'true' : 'false'}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: input.items ? JSON.stringify({ items: input.items }) : undefined,
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `cleanup plan failed: ${res.status}`),
    )
  }
  return (await res.json()) as JanitorPlanReport
}

/** Full dry run that fills the checklist; only runs while the drawer is open. */
export function useJanitorPlan(enabled: boolean) {
  return useQuery({
    queryKey: janitorPlanKeys.plan,
    queryFn: () => runPlan({ dryRun: true }),
    enabled,
    staleTime: 0,
    gcTime: 0,
    refetchOnWindowFocus: false,
    retry: false,
  })
}

/** Preview (dry run) or apply with the current allowlist. */
export function useJanitorPlanRun() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: runPlan,
    onSuccess: (_report, input) => {
      if (!input.dryRun) {
        void queryClient.invalidateQueries({ queryKey: ['janitor'] })
      }
    },
  })
}

async function fetchHistory(): Promise<JanitorPlanReport[]> {
  const res = await fetch('/api/v1/system/janitor/runs?limit=20')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `load cleanup history failed: ${res.status}`),
    )
  }
  return (await res.json()) as JanitorPlanReport[]
}

export function useJanitorPlanHistory(enabled: boolean) {
  return useQuery({
    queryKey: janitorPlanKeys.history,
    queryFn: fetchHistory,
    enabled,
  })
}
