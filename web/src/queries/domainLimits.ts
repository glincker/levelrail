// GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/limits
// (internal/api/domain_limits.go): per-domain body cap and timeouts.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DomainLimits {
  domain: string
  configured: boolean
  max_body_bytes: number
  read_timeout_seconds: number
  write_timeout_seconds: number
  min_body_bytes: number
  ceiling_body_bytes: number
  ceiling_timeout_seconds: number
  updated_at?: string
}

export interface DomainLimitsInput {
  max_body_bytes: number
  read_timeout_seconds: number
  write_timeout_seconds: number
}

export const domainLimitsKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'limits'] as const,
}

function path(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/limits`
}

async function send(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<DomainLimits> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainLimits
}

export function domainLimitsQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainLimitsKeys.detail(appName, domain),
    queryFn: () => send(path(appName, domain), undefined, 'fetch limits'),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainLimits(appName: string, domain: string) {
  return useQuery(domainLimitsQueryOptions(appName, domain))
}

export function useSetDomainLimits(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainLimits, ApiError, DomainLimitsInput>({
    mutationFn: (input) =>
      send(
        path(appName, domain),
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        },
        'update limits',
      ),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainLimitsKeys.detail(appName, domain),
        updated,
      )
    },
  })
}

export function useResetDomainLimits(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(path(appName, domain), { method: 'DELETE' })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `reset limits failed: ${res.status}`),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: domainLimitsKeys.detail(appName, domain),
      }),
  })
}
