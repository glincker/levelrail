import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { staticSitesKeys } from './staticSites'

export interface SiteHook {
  enabled: boolean
  created_at?: string
  /** Set only on create and rotate, the one time the secret URL is visible. */
  url?: string
}

const root = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}/hook`

const hookKey = (name: string) =>
  [...staticSitesKeys.all, 'hook', name] as const

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function siteHookQueryOptions(name: string) {
  return queryOptions({
    queryKey: hookKey(name),
    queryFn: async (): Promise<SiteHook> => {
      const res = await fetch(root(name))
      if (!res.ok) return fail(res, 'hook')
      return (await res.json()) as SiteHook
    },
    staleTime: 30_000,
    retry: false,
  })
}

export function useSiteHook(name: string) {
  return useQuery(siteHookQueryOptions(name))
}

/** Create, or rotate when the variable is true; the server answers 409 if the intent no longer matches. */
export function useMintSiteHook(name: string) {
  const qc = useQueryClient()
  return useMutation<SiteHook, ApiError, boolean>({
    mutationFn: async (rotate) => {
      const res = await fetch(root(name), {
        method: 'POST',
        ...(rotate
          ? {
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ rotate: true }),
            }
          : {}),
      })
      if (!res.ok) return fail(res, 'hook')
      return (await res.json()) as SiteHook
    },
    onSettled: () => qc.invalidateQueries({ queryKey: hookKey(name) }),
  })
}

export function useRevokeSiteHook(name: string) {
  const qc = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(root(name), { method: 'DELETE' })
      if (!res.ok) return fail(res, 'revoke')
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: hookKey(name) }),
  })
}
