// Matches internal/api/vitals.go: histogram-derived percentiles per metric and route.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'
import { apiRequest } from '../lib/apiRequest'

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
  overflow?: boolean
  p95_overflow?: boolean
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

function request(
  app: string,
  method: string,
  suffix = '',
  body?: unknown,
): Promise<AppVitals> {
  return apiRequest<AppVitals>(
    `/api/v1/apps/${encodeURIComponent(app)}/vitals${suffix}`,
    { method, body, fallbackMessage: 'web vitals request failed' },
  )
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
