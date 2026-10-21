import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  BranchDeploys,
  BranchDeploySettingsUpdate,
} from '../types/branchDeploy'

export const branchDeployKeys = {
  all: ['branch-deploys'] as const,
  detail: (appName: string) => [...branchDeployKeys.all, appName] as const,
}

const DEPLOYING_POLL_INTERVAL_MS = 3_000

function baseUrl(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/branch-deploys`
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function fetchBranchDeploys(
  appName: string,
): Promise<BranchDeploys> {
  const res = await fetch(baseUrl(appName))
  if (!res.ok) throw await failure(res, 'fetch branch deploys')
  return (await res.json()) as BranchDeploys
}

export function branchDeploysQueryOptions(appName: string) {
  return queryOptions({
    queryKey: branchDeployKeys.detail(appName),
    queryFn: () => fetchBranchDeploys(appName),
    refetchInterval: (query) =>
      query.state.data?.deploys.some((d) => d.status === 'deploying')
        ? DEPLOYING_POLL_INTERVAL_MS
        : false,
  })
}

export function useBranchDeploys(appName: string) {
  return useQuery(branchDeploysQueryOptions(appName))
}

export function useSetBranchDeploySettings(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<BranchDeploys, ApiError, BranchDeploySettingsUpdate>({
    mutationFn: async (update) => {
      const res = await fetch(baseUrl(appName), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(update),
      })
      if (!res.ok) throw await failure(res, 'save branch deploy settings')
      return (await res.json()) as BranchDeploys
    },
    onSuccess: (data) => {
      queryClient.setQueryData(branchDeployKeys.detail(appName), data)
    },
  })
}

export function useDeleteBranchDeploy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (slug) => {
      const res = await fetch(
        `${baseUrl(appName)}/${encodeURIComponent(slug)}`,
        {
          method: 'DELETE',
        },
      )
      if (!res.ok && res.status !== 207) {
        throw await failure(res, 'delete branch deploy')
      }
    },
    onSettled: () =>
      queryClient.invalidateQueries({
        queryKey: branchDeployKeys.detail(appName),
      }),
  })
}
