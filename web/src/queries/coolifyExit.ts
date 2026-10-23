// Coolify exit report (internal/api/coolify_exit.go): per-resource outcomes and the cutover checklist.

import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ExitOutcome = 'imported' | 'skipped' | 'manual'

export interface ExitRow {
  kind: string
  name: string
  target?: string
  outcome: ExitOutcome
  reasons?: string[]
  next?: string[]
}

export interface ExitStep {
  id: string
  title: string
  detail: string
  commands?: string[]
}

export interface ExitWave {
  number: number
  title: string
  why: string
  apps: { name: string; domains?: string[] }[]
}

export interface CoolifyExitReport {
  mode: 'dry-run' | 'verify'
  rows: ExitRow[]
  counts: Partial<Record<ExitOutcome, number>>
  waves: ExitWave[]
  checklist: ExitStep[]
  notes?: string[]
  ready_to_retire: boolean
}

export const coolifyExitKeys = {
  one: (id: string) => ['migration', 'apps', 'exit-report', id] as const,
}

export function useCoolifyExitReport(id: string, enabled: boolean) {
  return useQuery<CoolifyExitReport, ApiError>({
    queryKey: coolifyExitKeys.one(id),
    enabled,
    retry: false,
    queryFn: async () => {
      const res = await fetch(
        `/api/v1/migration/apps/sessions/${encodeURIComponent(id)}/exit-report`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `request failed: ${res.status}`),
        )
      }
      return (await res.json()) as CoolifyExitReport
    },
  })
}
