import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

// GET /api/v1/apps/{name}/rollout.
export interface RolloutPolicy {
  max_surge?: string
  max_unavailable?: string
  min_available?: string
}

export interface RolloutResolved {
  max_surge: number
  max_unavailable: number
  min_available: number
}

export interface RolloutProgress {
  active: boolean
  phase: 'rolling' | 'complete' | 'failed'
  wave: number
  waves: number
  desired: number
  updated: number
  ready: number
  unavailable: number
  surge: number
  max_surge: number
  max_unavailable: number
  message?: string
  started_at?: string
  updated_at?: string
}

export interface AppRollout {
  app: string
  strategy: string
  replicas: number
  policy?: RolloutPolicy
  resolved?: RolloutResolved
  progress: RolloutProgress | null
}

export const appRolloutKeys = {
  rollout: (appName: string) =>
    [...appKeys.detail(appName), 'rollout'] as const,
}

/** A 404 means an older server without the route: show nothing. */
export async function fetchAppRollout(
  appName: string,
): Promise<AppRollout | null> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/rollout`)
  if (res.status === 404) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app rollout failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppRollout
}

const POLL_MS = Number(import.meta.env.VITE_ROLLOUT_POLL_MS) || 3000

export function appRolloutQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appRolloutKeys.rollout(appName),
    queryFn: () => fetchAppRollout(appName),
    refetchInterval: (query) =>
      query.state.data?.progress?.active ? POLL_MS : false,
  })
}

export function useAppRollout(appName: string) {
  return useQuery(appRolloutQueryOptions(appName))
}

/** PUT replaces the saved policy; an empty field falls back to the env default. */
export function useSaveRolloutPolicy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<RolloutPolicy, ApiError, RolloutPolicy>({
    mutationFn: async (policy) => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/rollout`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(policy),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `save rollout policy failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as RolloutPolicy
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: appRolloutKeys.rollout(appName),
      }),
  })
}

export function useResetRolloutPolicy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/rollout`,
        { method: 'DELETE' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `reset rollout policy failed: ${res.status}`,
          ),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: appRolloutKeys.rollout(appName),
      }),
  })
}
