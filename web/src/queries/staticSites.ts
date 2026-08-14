// Query-key factory and fetcher for GET /api/v1/static-sites
// (internal/api/static_sites.go's handleListStaticSites): dashboard
// visibility for build.type: static sites, served
// directly by embedded Caddy with no container, previously nowhere in
// the API or the dashboard at all. Mirrors certificates.ts's own shape:
// an ordinary suspense query, requires a session.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const staticSitesKeys = {
  all: ['static-sites'] as const,
}

// StaticSite mirrors internal/api/static_sites.go's staticSiteResource
// wire shape exactly. root_dir is deliberately absent: it's an absolute
// filesystem path on the control plane host with no frontend use, see
// that file's own doc comment for the full reasoning.
export interface StaticSite {
  name: string
  domains: string[]
}

export async function fetchStaticSites(): Promise<StaticSite[]> {
  const res = await fetch('/api/v1/static-sites')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch static sites failed: ${res.status}`),
    )
  }
  return (await res.json()) as StaticSite[]
}

// staleTime matches certificatesQueryOptions's own reasoning: this is an
// occasionally-checked summary, not a live-updating view, a fresh read
// per navigation is enough.
export function staticSitesQueryOptions() {
  return queryOptions({
    queryKey: staticSitesKeys.all,
    queryFn: fetchStaticSites,
    staleTime: 60_000,
  })
}

export function useStaticSites() {
  return useSuspenseQuery(staticSitesQueryOptions())
}
