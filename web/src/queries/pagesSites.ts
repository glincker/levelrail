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
