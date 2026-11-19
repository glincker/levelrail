// Pages (static sites) list with the optional fields the static-sites API
// grows into, plus the create call. Fields that the server does not send
// yet stay undefined and the UI shows a dash rather than a guess.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { staticSitesKeys, type StaticSite } from './staticSites'

export interface PagesSite extends StaticSite {
  last_deploy_at?: string
  size_bytes?: number
}

export const pagesSitesKeys = {
  list: [...staticSitesKeys.all, 'pages'] as const,
}

export async function fetchPagesSites(): Promise<PagesSite[]> {
  const res = await fetch('/api/v1/static-sites')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch static sites failed: ${res.status}`),
    )
  }
  return ((await res.json()) as PagesSite[] | null) ?? []
}

export function pagesSitesQueryOptions() {
  return queryOptions({
    queryKey: pagesSitesKeys.list,
    queryFn: fetchPagesSites,
    staleTime: 30_000,
  })
}

export function usePagesSites() {
  return useQuery(pagesSitesQueryOptions())
}

export type CreatePagesSiteResult = 'created' | 'fallback'

/** POST /api/v1/static-sites. A 404 or 405 means the server predates it: the caller then creates the site through the drop-publish path. */
export async function createPagesSite(
  name: string,
): Promise<CreatePagesSiteResult> {
  const res = await fetch('/api/v1/static-sites', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  if (res.status === 404 || res.status === 405) return 'fallback'
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create site failed: ${res.status}`),
    )
  }
  return 'created'
}

export interface SiteDomainRecord {
  type: string
  name: string
  value: string
}

export interface SiteDomainSetup {
  domain: string
  mode: 'auto' | 'manual'
  records: SiteDomainRecord[]
  verified: boolean
  cert: string
  detail?: string
}

const siteBase = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}/domains`

async function readSetup(
  res: Response,
  what: string,
): Promise<SiteDomainSetup> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as SiteDomainSetup
}

export async function addSiteDomain(
  name: string,
  domain: string,
): Promise<SiteDomainSetup> {
  const res = await fetch(siteBase(name), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ domain }),
  })
  return readSetup(res, 'add domain')
}

export async function fetchSiteDomainStatus(
  name: string,
  domain: string,
): Promise<SiteDomainSetup> {
  return readSetup(
    await fetch(`${siteBase(name)}/${encodeURIComponent(domain)}/status`),
    'domain status',
  )
}

export interface PagesSiteDeploy {
  id: string
  status: string
  source: string
  commit_sha?: string
  started_at: string
  finished_at?: string
  error?: string
}

export interface PagesSiteDetail {
  name: string
  domains: string[]
  published: boolean
  file_count: number
  size_bytes: number
  updated_at?: string
  latest_deploy?: PagesSiteDeploy
}

const siteRoot = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}`

async function readJson<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function pagesSiteDetailQueryOptions(name: string) {
  return queryOptions({
    queryKey: [...staticSitesKeys.all, 'pages-detail', name] as const,
    queryFn: async () =>
      readJson<PagesSiteDetail>(await fetch(siteRoot(name)), 'site detail'),
    staleTime: 30_000,
    retry: false,
  })
}

/** Published size and latest deploy; `enabled` lets a list row skip it when the list already carried the values. */
export function usePagesSiteDetail(name: string, enabled = true) {
  return useQuery({ ...pagesSiteDetailQueryOptions(name), enabled })
}

export type CheckStatus = 'ok' | 'warn' | 'fail'

export interface SiteCheck {
  id: string
  status: CheckStatus
  title: string
  detail: string
  path?: string
}

export interface SiteChecksReport {
  name: string
  score: number
  files_scanned: number
  truncated: boolean
  total_bytes: number
  checks: SiteCheck[]
}

export const siteChecksKey = (name: string) =>
  [...staticSitesKeys.all, 'checks', name] as const

export function siteChecksQueryOptions(name: string) {
  return queryOptions({
    queryKey: siteChecksKey(name),
    queryFn: async () =>
      readJson<SiteChecksReport>(
        await fetch(`${siteRoot(name)}/checks`),
        'site checks',
      ),
    staleTime: 60_000,
    retry: false,
  })
}

export type TrafficWindow = '24h' | '7d'

export interface TrafficCount {
  label: string
  count: number
}

export interface SiteTraffic {
  name: string
  window: TrafficWindow
  since: string
  generated_at: string
  requests: number
  visitors: number
  bytes_out: number
  status: { '2xx': number; '3xx': number; '4xx': number; '5xx': number }
  top_paths: TrafficCount[]
  top_404_paths: TrafficCount[]
  partial: boolean
  partial_notes?: string[]
}

/** null means the server has no edge access source (501), shown as an unavailable note rather than an error. */
export function siteTrafficQueryOptions(name: string, window: TrafficWindow) {
  return queryOptions({
    queryKey: [...staticSitesKeys.all, 'traffic', name, window] as const,
    queryFn: async (): Promise<SiteTraffic | null> => {
      const res = await fetch(`${siteRoot(name)}/traffic?window=${window}`)
      if (res.status === 501) return null
      return readJson<SiteTraffic>(res, 'site traffic')
    },
    staleTime: 60_000,
    retry: false,
  })
}
