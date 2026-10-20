// GET/PUT /api/v1/apps/{name}/domains/{domain}/search-visibility
// (internal/api/domain_search_visibility.go): whether a domain asks
// search engines to stay away.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DomainSearchVisibility {
  domain: string
  hidden: boolean
}

export const domainSearchVisibilityKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'search-visibility'] as const,
}

function path(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/search-visibility`
}

async function fetchDomainSearchVisibility(
  appName: string,
  domain: string,
): Promise<DomainSearchVisibility> {
  const res = await fetch(path(appName, domain))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch search visibility failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainSearchVisibility
}

export function domainSearchVisibilityQueryOptions(
  appName: string,
  domain: string,
) {
  return queryOptions({
    queryKey: domainSearchVisibilityKeys.detail(appName, domain),
    queryFn: () => fetchDomainSearchVisibility(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainSearchVisibility(appName: string, domain: string) {
  return useQuery(domainSearchVisibilityQueryOptions(appName, domain))
}

export function useSetDomainSearchVisibility(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainSearchVisibility, ApiError, boolean>({
    mutationFn: async (hidden) => {
      const res = await fetch(path(appName, domain), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ hidden }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `update search visibility failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as DomainSearchVisibility
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainSearchVisibilityKeys.detail(appName, domain),
        updated,
      )
    },
  })
}
