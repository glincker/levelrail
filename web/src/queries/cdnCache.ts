// CDN and cache views: internal/api/cache_app.go and cdn_overview.go.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type CdnWindow = '5m' | '1h' | '24h'
export type PurgeScope = 'url' | 'prefix' | 'tag' | 'all'

export interface CacheBucket {
  minute: number
  hits: number
  misses: number
  hit_bytes: number
  miss_bytes: number
}

export interface CacheNumbers {
  hits: number
  misses: number
  stale: number
  bypasses: number
  hit_rate: number
  hit_bytes: number
  origin_bytes: number
  entries: number
  bytes: number
}

export interface CacheLimits {
  max_bytes: number
  used_bytes: number
  max_object_bytes: number
  tag_header: string
  location: string
}

export interface AppCacheDomain extends CacheNumbers {
  domain: string
  enabled: boolean
  rules: number
}

export interface AppCache {
  app: string
  limits: CacheLimits
  totals: CacheNumbers
  series: CacheBucket[]
  domains: AppCacheDomain[]
}

export interface SystemCacheApp extends CacheNumbers {
  app: string
  domains: number
  enabled_domains: number
  requests: number
}

export interface SystemCache {
  limits: CacheLimits
  totals: CacheNumbers
  series: CacheBucket[]
  apps: SystemCacheApp[]
}

export interface CdnPoint {
  time: string
  requests: number
  errors_5xx: number
}

export interface CdnNode {
  id: string
  name: string
  region?: string
  requests: number
  share: number
  apps: number
}

export interface CdnOverview {
  window: CdnWindow
  from: string
  to: string
  has_traffic: boolean
  requests: number
  rate_per_sec: number
  error_rate_5xx: number
  bytes_out: number
  series: CdnPoint[]
  cache: {
    covered_minutes: number
    hits: number
    misses: number
    hit_rate: number
    hit_bytes: number
    origin_bytes: number
    series: CacheBucket[]
  }
  nodes: CdnNode[]
  regions_labeled: boolean
}

export interface PurgeRecord {
  domain: string
  scope: PurgeScope
  value?: string
  removed: number
  actor: string
  at: string
}

export interface PurgeRequest {
  scope: PurgeScope
  value?: string
  domain?: string
}

export interface PurgeResult {
  removed: number
  domains: { domain: string; removed: number }[]
}

export interface PresetResult {
  domains: { domain: string; applied: boolean; rules: number }[]
}

export const cdnKeys = {
  all: ['cdn'] as const,
  system: ['cdn', 'system-cache'] as const,
  overview: (w: CdnWindow) => ['cdn', 'overview', w] as const,
  app: (app: string) => ['cdn', 'app', app] as const,
  purges: (app: string) => ['cdn', 'purges', app] as const,
}

const REFETCH_MS = 15_000

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url, { cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

async function postJson<T>(
  url: string,
  body: unknown,
  what: string,
): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

const appBase = (app: string) => `/api/v1/apps/${encodeURIComponent(app)}/cache`

export function systemCacheQueryOptions() {
  return queryOptions({
    queryKey: cdnKeys.system,
    queryFn: () => getJson<SystemCache>('/api/v1/system/cache', 'fetch cache'),
    refetchInterval: REFETCH_MS,
    retry: false,
  })
}

export function useSystemCache() {
  return useQuery(systemCacheQueryOptions())
}

export function useCdnOverview(window: CdnWindow) {
  return useQuery({
    queryKey: cdnKeys.overview(window),
    queryFn: () =>
      getJson<CdnOverview>(
        `/api/v1/cdn/overview?window=${window}`,
        'fetch CDN overview',
      ),
    refetchInterval: REFETCH_MS,
    retry: false,
  })
}

export function useAppCache(app: string) {
  return useQuery({
    queryKey: cdnKeys.app(app),
    queryFn: () => getJson<AppCache>(appBase(app), 'fetch app cache'),
    refetchInterval: REFETCH_MS,
    retry: false,
  })
}

export function useCachePurges(app: string) {
  return useQuery({
    queryKey: cdnKeys.purges(app),
    queryFn: async () =>
      (
        await getJson<{ purges: PurgeRecord[] }>(
          `${appBase(app)}/purges`,
          'fetch purge history',
        )
      ).purges,
    retry: false,
  })
}

export function usePurgeAppCache(app: string) {
  const queryClient = useQueryClient()
  return useMutation<PurgeResult, ApiError, PurgeRequest>({
    mutationFn: (req) =>
      postJson<PurgeResult>(`${appBase(app)}/purge`, req, 'purge cache'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: cdnKeys.all }),
  })
}

export function useApplyCachePreset(app: string) {
  const queryClient = useQueryClient()
  return useMutation<PresetResult, ApiError, string | undefined>({
    mutationFn: (domain) =>
      postJson<PresetResult>(
        `${appBase(app)}/preset`,
        domain ? { domain } : {},
        'apply cache preset',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: cdnKeys.all })
      void queryClient.invalidateQueries({ queryKey: ['apps'] })
    },
  })
}
