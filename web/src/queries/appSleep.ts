// /api/v1/apps/{name}/sleep (internal/api/app_sleep.go): stop an idle app and
// wake it on the next request.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useQuery,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AppSleep {
  enabled: boolean
  idle_minutes: number
  sleeping: boolean
  // When the app went to sleep, set only while it is asleep.
  since?: string
  hold_requests: boolean
  // Why the app cannot sleep.
  blocked_reason?: 'pinned_port'
}

export const appSleepKeys = {
  detail: (appName: string) => [...appKeys.detail(appName), 'sleep'] as const,
}

async function request(
  appName: string,
  method: string,
  suffix: string,
  body?: unknown,
): Promise<AppSleep> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/sleep${suffix}`,
    {
      method,
      headers: body ? { 'Content-Type': 'application/json' } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `sleep request failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppSleep
}

export function appSleepQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appSleepKeys.detail(appName),
    queryFn: () => request(appName, 'GET', ''),
  })
}

export function useAppSleep(appName: string) {
  return useSuspenseQuery(appSleepQueryOptions(appName))
}

export function useAppSleepActions(appName: string) {
  const queryClient = useQueryClient()
  const onSuccess = (result: AppSleep) => {
    queryClient.setQueryData(appSleepKeys.detail(appName), result)
    void queryClient.invalidateQueries({
      queryKey: [...appKeys.detail(appName), 'sleep-history'],
    })
    void queryClient.invalidateQueries({
      queryKey: appKeys.detail(appName),
      exact: true,
    })
  }
  const setIdle = useMutation<
    AppSleep,
    ApiError,
    { idle_minutes: number; hold_requests?: boolean }
  >({
    mutationFn: (body) => request(appName, 'PUT', '', body),
    onSuccess,
  })
  const wake = useMutation<AppSleep, ApiError, void>({
    mutationFn: () => request(appName, 'POST', '/wake'),
    onSuccess,
  })
  const sleepNow = useMutation<AppSleep, ApiError, void>({
    mutationFn: () => request(appName, 'POST', '/now'),
    onSuccess,
  })
  return { setIdle, wake, sleepNow }
}

/** useAppAsleep reports whether the app is asleep, without suspending or failing the page. */
export function useAppAsleep(appName: string): boolean {
  const { data } = useQuery({
    ...appSleepQueryOptions(appName),
    staleTime: 15_000,
    retry: false,
  })
  return data?.sleeping ?? false
}
