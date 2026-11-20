import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'

export interface PagesDeploy {
  id: string
  status: string
  url?: string
  live: boolean
  promotable: boolean
}

export const pagesDeployKeys = {
  list: (name: string) => [...appKeys.detail(name), 'pages-deploys'] as const,
}

export function pagesDeploysQueryOptions(name: string) {
  return queryOptions({
    queryKey: pagesDeployKeys.list(name),
    queryFn: async (): Promise<PagesDeploy[]> => {
      const res = await fetch(
        `/api/v1/static-sites/${encodeURIComponent(name)}/deploys`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `list deploys failed: ${res.status}`),
        )
      }
      const body = (await res.json()) as { deploys?: PagesDeploy[] }
      return body.deploys ?? []
    },
  })
}

/** Makes a retained deploy the live one, the same effect as a rollback to it. */
export function usePromoteDeploy(name: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (deployId) => {
      const res = await fetch(
        `/api/v1/static-sites/${encodeURIComponent(name)}/deploys/${encodeURIComponent(deployId)}/promote`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `promote failed: ${res.status}`),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: appKeys.detail(name) }),
  })
}
