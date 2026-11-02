// Per-app Cloudflare proxy override: internal/api/app_cdn.go.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AppCdn {
  app: string
  /** null means the app follows the server-wide setting. */
  proxied: boolean | null
  global_proxied: boolean
  effective: boolean
}

const url = (app: string) => `/api/v1/apps/${encodeURIComponent(app)}/cdn`
const key = (app: string) => ['apps', app, 'cdn'] as const

export function useAppCdn(app: string) {
  return useQuery<AppCdn, ApiError>({
    queryKey: key(app),
    enabled: app.length > 0,
    staleTime: 15_000,
    queryFn: async () => {
      const res = await fetch(url(app))
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `load CDN setting failed: ${res.status}`),
        )
      }
      return (await res.json()) as AppCdn
    },
  })
}

export function useSetAppCdn(app: string) {
  const queryClient = useQueryClient()
  return useMutation<AppCdn, ApiError, boolean | null>({
    mutationFn: async (proxied) => {
      const res = await fetch(url(app), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ proxied }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `save CDN setting failed: ${res.status}`),
        )
      }
      return (await res.json()) as AppCdn
    },
    onSuccess: (data) => {
      queryClient.setQueryData(key(app), data)
    },
  })
}
