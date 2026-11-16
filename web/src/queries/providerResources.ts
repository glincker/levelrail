import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  ProviderResources,
  ProviderSpend,
} from '../types/providerResources'
import { RateLimitedError } from './providerCapabilities'

export const providerResourcesKey = (provider: string) =>
  ['node-providers', provider, 'resources'] as const
export const providerSpendKey = (provider: string) =>
  ['node-providers', provider, 'spend'] as const

async function fetchProviderRead<T>(
  provider: string,
  path: 'resources' | 'spend',
  refresh: boolean,
): Promise<T> {
  const res = await fetch(
    `/api/v1/node-providers/${encodeURIComponent(provider)}/${path}${refresh ? '?refresh=1' : ''}`,
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
      await readErrorMessage(res, `fetch ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useProviderResources(provider: string, enabled: boolean) {
  return useQuery(
    queryOptions({
      queryKey: providerResourcesKey(provider),
      queryFn: () =>
        fetchProviderRead<ProviderResources>(provider, 'resources', false),
      enabled,
      retry: false,
    }),
  )
}

export function useRefreshProviderResources(provider: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: () =>
      fetchProviderRead<ProviderResources>(provider, 'resources', true),
    onSuccess: (data) => {
      qc.setQueryData(providerResourcesKey(provider), data)
    },
  })
}

export function useProviderSpend(provider: string, enabled: boolean) {
  return useQuery(
    queryOptions({
      queryKey: providerSpendKey(provider),
      queryFn: () => fetchProviderRead<ProviderSpend>(provider, 'spend', false),
      enabled,
      retry: false,
    }),
  )
}

export function useRefreshProviderSpend(provider: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: () => fetchProviderRead<ProviderSpend>(provider, 'spend', true),
    onSuccess: (data) => {
      qc.setQueryData(providerSpendKey(provider), data)
    },
  })
}
