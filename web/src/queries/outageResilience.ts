import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ResilienceStatus = 'pass' | 'fail' | 'warn' | 'unknown'

export interface ResilienceCheck {
  id: string
  status: ResilienceStatus
  detail: string
  fix?: string
}

export interface ResilienceScope {
  kind: 'ingress' | 'node'
  id: string
  name: string
  checks: ResilienceCheck[]
}

export interface ResilienceReport {
  generated_at: string
  simulated: boolean
  overall: ResilienceStatus
  scopes: ResilienceScope[]
}

export const resilienceKey = ['system', 'resilience'] as const

async function parse(res: Response, what: string): Promise<ResilienceReport> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as ResilienceReport
}

export function useResilience() {
  return useQuery({
    queryKey: resilienceKey,
    queryFn: async () =>
      parse(await fetch('/api/v1/system/resilience'), 'resilience check'),
    retry: false,
  })
}

function useReportMutation(url: string, what: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => parse(await fetch(url, { method: 'POST' }), what),
    onSuccess: (data) => qc.setQueryData(resilienceKey, data),
  })
}

export function useSimulateOutage() {
  return useReportMutation(
    '/api/v1/system/resilience/simulate',
    'outage simulation',
  )
}

export function useFixSnapshot() {
  return useReportMutation(
    '/api/v1/system/resilience/snapshot',
    'save route snapshot',
  )
}
