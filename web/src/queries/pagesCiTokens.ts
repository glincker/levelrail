import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { staticSitesKeys } from './staticSites'

export interface SiteDeployToken {
  id: string
  name: string
  created_at: string
  expires_at?: string
  last_used_at?: string
  revoked_at?: string
  /** Set only on create, the one time the secret is visible. */
  token?: string
}

const root = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}/deploy-tokens`

const key = (name: string) =>
  [...staticSitesKeys.all, 'deploy-tokens', name] as const

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function siteDeployTokensQueryOptions(name: string) {
  return queryOptions({
    queryKey: key(name),
    queryFn: async (): Promise<SiteDeployToken[]> => {
      const res = await fetch(root(name))
      if (!res.ok) return fail(res, 'tokens')
      return (await res.json()) as SiteDeployToken[]
    },
    staleTime: 30_000,
    retry: false,
  })
}

export function useSiteDeployTokens(name: string) {
  return useQuery(siteDeployTokensQueryOptions(name))
}

export function useCreateSiteDeployToken(name: string) {
  const qc = useQueryClient()
  return useMutation<SiteDeployToken, ApiError, string>({
    mutationFn: async (label) => {
      const res = await fetch(root(name), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: label }),
      })
      if (!res.ok) return fail(res, 'token')
      return (await res.json()) as SiteDeployToken
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key(name) }),
  })
}

export function useRevokeSiteDeployToken(name: string) {
  const qc = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (id) => {
      const res = await fetch(`${root(name)}/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      if (!res.ok) return fail(res, 'revoke')
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: key(name) }),
  })
}
