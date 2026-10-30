// Matches internal/api/app_urls.go: every way to reach one app right now.
import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollUnlessMissing } from '../lib/pollUnlessMissing'

export type AppUrlKind = 'loopback' | 'lan' | 'public' | 'domain' | 'internal'

export type ReachableHint =
  'reachable' | 'not_running' | 'bound_localhost' | 'remote_node' | 'unknown'

export interface AppUrlAddress {
  kind: AppUrlKind
  host?: string
  url?: string
  port?: number
  reachable_hint: ReachableHint
}

export interface AppUrls {
  app: string
  container_port: number
  host_port?: number
  bind_address: string
  running: boolean
  node?: string
  local_node: boolean
  addresses: AppUrlAddress[]
  http_fallback_url?: string
}

export const appUrlsKeys = {
  detail: (app: string) => ['apps', app, 'urls'] as const,
}

export async function fetchAppUrls(app: string): Promise<AppUrls> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(app)}/urls`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app urls failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppUrls
}

export function appUrlsQueryOptions(app: string) {
  return queryOptions({
    queryKey: appUrlsKeys.detail(app),
    queryFn: () => fetchAppUrls(app),
  })
}

export function useAppUrls(app: string) {
  return useQuery({
    ...appUrlsQueryOptions(app),
    refetchInterval: pollUnlessMissing(15_000),
  })
}
