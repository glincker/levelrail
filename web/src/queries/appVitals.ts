// Matches internal/api/vitals.go: histogram-derived percentiles per metric and route.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type VitalsMetric = 'LCP' | 'INP' | 'CLS' | 'TTFB' | 'FCP'
export type VitalsRating = 'good' | 'needs-improvement' | 'poor'

export const VITALS_METRICS: VitalsMetric[] = [
  'LCP',
  'INP',
  'CLS',
  'TTFB',
  'FCP',
]

export interface VitalsStat {
  count: number
  p50: number
  p75: number
  p95: number
  rating: VitalsRating | ''
}

export interface VitalsRoute {
  route: string
  metrics: Partial<Record<VitalsMetric, VitalsStat>>
}

export interface AppVitals {
  enabled: boolean
  public_key: string
  days: number
  overall: Partial<Record<VitalsMetric, VitalsStat>>
  routes: VitalsRoute[]
}

export const appVitalsKeys = {
  detail: (app: string) => ['apps', app, 'vitals'] as const,
}

async function request(
  app: string,
  method: string,
  suffix = '',
  body?: unknown,
): Promise<AppVitals> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(app)}/vitals${suffix}`,
    {
      method,
      headers: body ? { 'Content-Type': 'application/json' } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `web vitals request failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppVitals
}

export function useAppVitals(app: string) {
  return useQuery({
    queryKey: appVitalsKeys.detail(app),
    queryFn: () => request(app, 'GET'),
  })
}

export function useAppVitalsActions(app: string) {
  const qc = useQueryClient()
  const onSuccess = (data: AppVitals) =>
    qc.setQueryData(appVitalsKeys.detail(app), data)
  const setEnabled = useMutation<AppVitals, ApiError, boolean>({
    mutationFn: (enabled) => request(app, 'PUT', '', { enabled }),
    onSuccess,
  })
  const rotate = useMutation<AppVitals, ApiError, void>({
    mutationFn: () => request(app, 'POST', '/rotate-key'),
    onSuccess,
  })
  return { setEnabled, rotate }
}

/** Formats a p75 value: CLS is unitless, the rest are milliseconds. */
export function formatVitalValue(metric: VitalsMetric, value: number): string {
  return metric === 'CLS' ? String(value) : `${Math.round(value)} ms`
}
