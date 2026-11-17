import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { ProviderCapabilities } from '../lib/providerCapabilities'

export class RateLimitedError extends Error {
  readonly retryAfterSeconds: number
  constructor(message: string, retryAfterSeconds: number) {
    super(message)
    this.name = 'RateLimitedError'
    this.retryAfterSeconds = retryAfterSeconds
  }
}

export const providerCapabilitiesKey = (provider: string) =>
  ['node-providers', provider, 'capabilities'] as const

async function fetchCapabilities(
  provider: string,
  refresh: boolean,
): Promise<ProviderCapabilities> {
  const res = await fetch(
    `/api/v1/node-providers/${encodeURIComponent(provider)}/capabilities${refresh ? '?refresh=1' : ''}`,
  )
  if (res.status === 429) {
    const wait = Number(res.headers.get('Retry-After'))
    throw new RateLimitedError(
      await readErrorMessage(res, 'rate limited'),
      Number.isFinite(wait) && wait > 0 ? wait : 0,
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch capabilities failed: ${res.status}`),
    )
  }
  return (await res.json()) as ProviderCapabilities
}

export function useProviderCapabilities(provider: string, enabled: boolean) {
  return useQuery(
    queryOptions({
      queryKey: providerCapabilitiesKey(provider),
      queryFn: () => fetchCapabilities(provider, false),
      enabled,
    }),
  )
}

// Re-check is root-only and rate limited server side; a 429 surfaces as RateLimitedError.
export function useRecheckProviderCapabilities(provider: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: () => fetchCapabilities(provider, true),
    onSuccess: (data) => {
      qc.setQueryData(providerCapabilitiesKey(provider), data)
    },
  })
}
