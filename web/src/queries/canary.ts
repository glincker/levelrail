// /api/v1/apps/{name}/canary (internal/api/canary.go): run a new image beside
// the app and send it a share of traffic before promoting it.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface Canary {
  active: boolean
  image?: string
  weight: number
  created_at?: string
}

export const canaryKeys = {
  detail: (appName: string) => [...appKeys.detail(appName), 'canary'] as const,
}

async function request(
  appName: string,
  method: string,
  suffix: string,
  body?: unknown,
): Promise<Canary> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/canary${suffix}`,
    {
      method,
      headers: body ? { 'Content-Type': 'application/json' } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `canary request failed: ${res.status}`),
    )
  }
  return (await res.json()) as Canary
}

export function canaryQueryOptions(appName: string) {
  return queryOptions({
    queryKey: canaryKeys.detail(appName),
    queryFn: () => request(appName, 'GET', ''),
  })
}

export function useCanary(appName: string) {
  return useSuspenseQuery(canaryQueryOptions(appName))
}

export function useCanaryActions(appName: string) {
  const queryClient = useQueryClient()
  const onSuccess = (result: Canary) => {
    queryClient.setQueryData(canaryKeys.detail(appName), result)
    void queryClient.invalidateQueries({
      queryKey: appKeys.detail(appName),
      exact: true,
    })
  }
  const start = useMutation<
    Canary,
    ApiError,
    { image: string; weight: number }
  >({
    mutationFn: (body) => request(appName, 'POST', '', body),
    onSuccess,
  })
  const setWeight = useMutation<Canary, ApiError, number>({
    mutationFn: (weight) => request(appName, 'PUT', '', { weight }),
    onSuccess,
  })
  const promote = useMutation<Canary, ApiError, void>({
    mutationFn: () => request(appName, 'POST', '/promote'),
    onSuccess,
  })
  const abort = useMutation<Canary, ApiError, void>({
    mutationFn: () => request(appName, 'DELETE', ''),
    onSuccess,
  })
  return { start, setWeight, promote, abort }
}
