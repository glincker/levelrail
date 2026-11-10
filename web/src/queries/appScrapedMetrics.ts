// Query keys and fetchers for an app's own scraped metrics
// (internal/api/routes_app_metrics.go): the scrape target and stored series.

import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { pollMs } from '@/lib/pollTier'

export interface ScrapeTarget {
  configured: boolean
  port: number
  path: string
  last_error?: string
  updated_at?: string
}

export interface ScrapedPoint {
  timestamp: string
  value: number
}

export interface ScrapedSeries {
  labels: string
  points: ScrapedPoint[]
}

export interface ScrapedMetrics {
  names?: string[]
  metric?: string
  series?: ScrapedSeries[]
}

const REFETCH_MS = pollMs('active')

export const scrapedMetricsKeys = {
  target: (app: string) => [...appKeys.detail(app), 'scrape-target'] as const,
  names: (app: string) => [...appKeys.detail(app), 'scraped-names'] as const,
  series: (app: string, metric: string, hours: number) =>
    [...appKeys.detail(app), 'scraped-series', metric, hours] as const,
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}`
}

async function readJSON<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export async function fetchScrapeTarget(app: string): Promise<ScrapeTarget> {
  return readJSON(
    await fetch(`${base(app)}/scrape-target`),
    'fetch scrape target',
  )
}

export async function saveScrapeTarget(
  app: string,
  port: number,
  path: string,
): Promise<ScrapeTarget> {
  const res = await fetch(`${base(app)}/scrape-target`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ port, path }),
  })
  return readJSON(res, 'save scrape target')
}

export async function fetchScrapedMetrics(
  app: string,
  metric?: string,
  from?: string,
): Promise<ScrapedMetrics> {
  const q = new URLSearchParams()
  if (metric && from) {
    q.set('metric', metric)
    q.set('from', from)
    q.set('to', new Date().toISOString())
  }
  const suffix = q.size > 0 ? `?${q.toString()}` : ''
  return readJSON(
    await fetch(`${base(app)}/scraped-metrics${suffix}`),
    'fetch scraped metrics',
  )
}

export function useScrapeTarget(app: string) {
  return useQuery({
    queryKey: scrapedMetricsKeys.target(app),
    queryFn: () => fetchScrapeTarget(app),
  })
}

export function useScrapedNames(app: string, enabled: boolean) {
  return useQuery({
    queryKey: scrapedMetricsKeys.names(app),
    queryFn: enabled ? () => fetchScrapedMetrics(app) : skipToken,
    refetchInterval: REFETCH_MS,
  })
}

export function useScrapedSeries(app: string, metric: string, hours: number) {
  return useQuery({
    queryKey: scrapedMetricsKeys.series(app, metric, hours),
    queryFn: metric
      ? () =>
          fetchScrapedMetrics(
            app,
            metric,
            new Date(Date.now() - hours * 3_600_000).toISOString(),
          )
      : skipToken,
    refetchInterval: REFETCH_MS,
  })
}

export function useSaveScrapeTarget(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { port: number; path: string }) =>
      saveScrapeTarget(app, v.port, v.path),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: scrapedMetricsKeys.target(app) })
    },
  })
}
