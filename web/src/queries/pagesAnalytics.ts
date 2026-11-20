import { queryOptions } from '@tanstack/react-query'

export type AnalyticsRange = '24h' | '7d' | '30d'

export interface AnalyticsCount {
  label: string
  count: number
}

export interface PagesAnalytics {
  site: string
  range: AnalyticsRange
  retention_days: number
  requests: number
  top_pages: AnalyticsCount[]
  top_referrers: AnalyticsCount[]
  status: { code: number; count: number }[]
  partial: boolean
}

/** null means no edge access source (501) or a range beyond retention (400); the panel hides itself. */
export function pagesAnalyticsQueryOptions(
  name: string,
  range: AnalyticsRange,
) {
  return queryOptions({
    queryKey: ['static-sites', 'analytics', name, range] as const,
    queryFn: async (): Promise<PagesAnalytics | null> => {
      const res = await fetch(
        `/api/v1/static-sites/${encodeURIComponent(name)}/analytics?range=${range}`,
      )
      if (res.status === 501 || res.status === 400) return null
      if (!res.ok) throw new Error(`site analytics failed: ${res.status}`)
      return (await res.json()) as PagesAnalytics
    },
    staleTime: 60_000,
    retry: false,
  })
}
