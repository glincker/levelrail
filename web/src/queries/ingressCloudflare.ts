// GET/PUT /api/v1/settings/ingress/cloudflare (internal/api/settings_ingress_cloudflare.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface IngressCloudflare {
  preset: 'none' | 'cloudflare'
  source: 'saved' | 'env' | 'default'
  ranges: number
  ranges_source: 'embedded' | 'fetched'
  last_refresh?: string
  last_error?: string
  refresh_interval: string
}

export const ingressCloudflareKey = ['ingress-cloudflare'] as const

async function readJSON(
  res: Response,
  what: string,
): Promise<IngressCloudflare> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as IngressCloudflare
}

export function ingressCloudflareQueryOptions() {
  return queryOptions({
    queryKey: ingressCloudflareKey,
    queryFn: async () =>
      readJSON(
        await fetch('/api/v1/settings/ingress/cloudflare'),
        'fetch cloudflare preset',
      ),
    staleTime: 30_000,
    retry: false,
  })
}

export function useIngressCloudflare() {
  return useQuery(ingressCloudflareQueryOptions())
}

// Enabling the preset trusts a header, so the switch sends the confirmation
// after showing the helper text; turning it off never needs one.
export function useSetIngressCloudflare() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (preset: IngressCloudflare['preset']) =>
      readJSON(
        await fetch('/api/v1/settings/ingress/cloudflare', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            preset,
            confirm_weakening: preset === 'cloudflare',
          }),
        }),
        'update cloudflare preset',
      ),
    onSuccess: (updated) => {
      queryClient.setQueryData(ingressCloudflareKey, updated)
    },
  })
}
