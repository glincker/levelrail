import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type FixKind = 'auto' | 'guided' | 'manual'
export type FixRisk = 'safe' | 'restarts' | 'exposes'
export type FixState = 'healthy' | 'offer' | 'demoted' | 'rate_limited'
export type FixOutcome =
  'fixed' | 'still_failing' | 'cannot_fix_here' | 'skipped'

export interface FixChange {
  label: string
  detail?: string
  bytes?: number
}

export interface FixParamOption {
  value: string
  label: string
}

export interface FixParam {
  name: string
  options: FixParamOption[]
  default?: string
  free: boolean
}

export interface FixAttempt {
  fix_id: string
  at: string
  actor: string
  outcome: FixOutcome
  detail: string
}

// Matches internal/fixit's View.
export interface Fix {
  id: string
  check_id: string
  subject: string
  title: string
  why: string
  risk: FixRisk
  reversible: boolean
  state: FixState
  base_kind: FixKind
  kind: FixKind
  failing: boolean
  detail: string
  reason?: string
  changes: FixChange[]
  command?: string
  param?: FixParam
  last_attempt?: FixAttempt
  attempts: FixAttempt[]
  blocked?: string
}

export interface FixResult {
  fix_id: string
  outcome: FixOutcome
  detail: string
  view: Fix
}

export const fixKeys = {
  all: ['fixes'] as const,
}

async function fetchFixes(): Promise<Fix[]> {
  const res = await fetch('/api/v1/fixes')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch fixes failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { fixes: Fix[] }
  return body.fixes
}

export function fixesQueryOptions() {
  return queryOptions({
    queryKey: fixKeys.all,
    queryFn: fetchFixes,
    staleTime: 10_000,
  })
}

export function useFixes() {
  return useQuery(fixesQueryOptions())
}

export function useFixForCheck(checkId: string): Fix | undefined {
  const { data } = useFixes()
  return data?.find((f) => f.check_id === checkId && f.failing)
}

export function useApplyFix() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (vars: {
      id: string
      params?: Record<string, string>
    }): Promise<FixResult> => {
      const res = await fetch(
        `/api/v1/fixes/${encodeURIComponent(vars.id)}/apply`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ params: vars.params ?? {} }),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `apply fix failed: ${res.status}`),
        )
      }
      return (await res.json()) as FixResult
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: fixKeys.all })
      void qc.invalidateQueries({ queryKey: ['mesh-settings'] })
      void qc.invalidateQueries({ queryKey: ['system-doctor'] })
    },
  })
}
