// GET/PUT /api/v1/settings/ingress/dns01 (internal/api/settings_ingress_dns01.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface IngressDns01 {
  enabled: boolean
  source: 'stored' | 'default'
  provider_connected: boolean
}

export const ingressDns01Key = ['ingress-dns01'] as const

async function readJSON(res: Response, what: string): Promise<IngressDns01> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as IngressDns01
}

export function ingressDns01QueryOptions() {
  return queryOptions({
    queryKey: ingressDns01Key,
    queryFn: async () =>
      readJSON(
        await fetch('/api/v1/settings/ingress/dns01'),
        'fetch dns challenge setting',
      ),
    staleTime: 30_000,
    retry: false,
  })
}

export function useIngressDns01() {
  return useQuery(ingressDns01QueryOptions())
}

// Optimistic: flip the cache first, roll back if the server refuses
// (400 when no DNS provider is connected).
export function useSetIngressDns01() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (enabled: boolean) =>
      readJSON(
        await fetch('/api/v1/settings/ingress/dns01', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled }),
        }),
        'update dns challenge setting',
      ),
    onMutate: async (enabled) => {
      await queryClient.cancelQueries({ queryKey: ingressDns01Key })
      const previous = queryClient.getQueryData<IngressDns01>(ingressDns01Key)
      if (previous) {
        queryClient.setQueryData<IngressDns01>(ingressDns01Key, {
          ...previous,
          enabled,
        })
      }
      return { previous }
    },
    onError: (_err, _enabled, ctx) => {
      if (ctx?.previous) {
        queryClient.setQueryData(ingressDns01Key, ctx.previous)
      }
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(ingressDns01Key, updated)
    },
  })
}
