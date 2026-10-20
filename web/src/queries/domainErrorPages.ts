// Query-key factory and fetcher for GET/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/error-pages
// (internal/api/domain_error_pages.go's domainErrorPagesResource): a
// domain's status-code-to-HTML mappings, served by the embedded Caddy
// ingress instead of Caddy's bare default error text or whatever the
// backend itself returned. Unlike queries/domainRedirect.ts's single
// value, this resource is a list: PUT upserts one status-code-to-body
// mapping at a time, DELETE removes one (or every mapping, with no
// status_code given).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// The fixed, small set of status codes this feature covers, mirroring
// internal/api's domainErrorPagesAllowedStatusCodes exactly.
export const DOMAIN_ERROR_PAGE_STATUS_CODES = [404, 500, 502, 503] as const

// Mirrors internal/api's domainErrorPageEntry wire shape exactly.
export interface DomainErrorPageEntry {
  status_code: number
  body: string
}

// Mirrors internal/api's domainErrorPagesResource wire shape exactly.
export interface DomainErrorPages {
  domain: string
  pages: DomainErrorPageEntry[]
}

// Mirrors internal/api's setDomainErrorPageRequest wire shape exactly.
export interface SetDomainErrorPageRequest {
  status_code: number
  body: string
}

export const domainErrorPagesKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'error-pages'] as const,
}

function errorPagesPath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/error-pages`
}

export async function fetchDomainErrorPages(
  appName: string,
  domain: string,
): Promise<DomainErrorPages> {
  const res = await fetch(errorPagesPath(appName, domain))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch domain error pages failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainErrorPages
}

export function domainErrorPagesQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainErrorPagesKeys.detail(appName, domain),
    queryFn: () => fetchDomainErrorPages(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainErrorPages(appName: string, domain: string) {
  return useQuery(domainErrorPagesQueryOptions(appName, domain))
}

async function setDomainErrorPage(
  appName: string,
  domain: string,
  req: SetDomainErrorPageRequest,
): Promise<DomainErrorPages> {
  const res = await fetch(errorPagesPath(appName, domain), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set domain error page failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainErrorPages
}

export function useSetDomainErrorPage(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainErrorPages, ApiError, SetDomainErrorPageRequest>({
    mutationFn: (req) => setDomainErrorPage(appName, domain, req),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainErrorPagesKeys.detail(appName, domain),
        updated,
      )
    },
  })
}

// clearDomainErrorPage removes one mapping (statusCode given) or every
// mapping (statusCode omitted), mirroring the CLI's own
// "clear [--code N]" shape.
async function clearDomainErrorPage(
  appName: string,
  domain: string,
  statusCode?: number,
): Promise<DomainErrorPages> {
  const path =
    statusCode === undefined
      ? errorPagesPath(appName, domain)
      : `${errorPagesPath(appName, domain)}?status_code=${statusCode}`
  const res = await fetch(path, { method: 'DELETE' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `clear domain error page failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainErrorPages
}

export function useClearDomainErrorPage(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainErrorPages, ApiError, number | undefined>({
    mutationFn: (statusCode) =>
      clearDomainErrorPage(appName, domain, statusCode),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainErrorPagesKeys.detail(appName, domain),
        updated,
      )
    },
  })
}
