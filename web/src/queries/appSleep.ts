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
  // Deprecated: single legacy blocker, superseded by blockers.
  blocked_reason?: string
  eligible?: boolean
  blockers?: SleepIssue[]
  warnings?: SleepIssue[]
  settings?: SleepSettings
  port_wake?: SleepPortWake
  activity?: SleepActivity
  cold_start_ms_p50?: number
  estimated_memory_freed_bytes?: number
}

export type SleepFixKind =
  | 'enable_port_wake'
  | 'remove_pinned_port'
  | 'set_hold_requests'
  | 'attach_domain'

export interface SleepFix {
  kind: SleepFixKind
  label: string
  method?: string
  path?: string
  body?: unknown
}

export interface SleepIssue {
  code: string
  title: string
  detail: string
  fix?: SleepFix
}

export interface SleepSettings {
  idle_minutes: number
  hold_requests: boolean
  ignore_paths: string[]
  port_wake: boolean
}

export interface SleepPortWake {
  supported: boolean
  active: boolean
  ports: number[]
}

export interface SleepActivity {
  last_request_at?: string
  last_network_activity_at?: string
  open_connections: number
  idle_for_seconds: number
  will_sleep_at?: string
}

export interface SleepUpdate {
  idle_minutes: number
  hold_requests?: boolean
  ignore_paths?: string[]
  port_wake?: boolean
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

const API_PREFIX = '/api/v1'

// Without a path the fix is derived from its kind where one call suffices.
function runFix(appName: string, fix: SleepFix): Promise<unknown> {
  if (fix.path) {
    const path = fix.path.startsWith('/') ? fix.path : `/${fix.path}`
    return send(
      path.startsWith(API_PREFIX) ? path : `${API_PREFIX}${path}`,
      fix.method ?? 'POST',
      fix.body,
    )
  }
  if (fix.kind === 'enable_port_wake') {
    return request(appName, 'PUT', '', fix.body ?? { port_wake: true })
  }
  if (fix.kind === 'set_hold_requests') {
    return request(appName, 'PUT', '', fix.body ?? { hold_requests: true })
  }
  return Promise.reject(new ApiError(400, 'this fix has no automatic action'))
}

async function send(
  path: string,
  method: string,
  body?: unknown,
): Promise<unknown> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `sleep fix failed: ${res.status}`),
    )
  }
  return res.status === 204 ? null : res.json().catch(() => null)
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
  const refetchEligibility = () =>
    queryClient.invalidateQueries({ queryKey: appSleepKeys.detail(appName) })
  const setIdle = useMutation<AppSleep, ApiError, SleepUpdate>({
    mutationFn: (body) => request(appName, 'PUT', '', body),
    onSuccess,
    onError: refetchEligibility,
  })
  const applyFix = useMutation<unknown, ApiError, SleepFix>({
    mutationFn: (fix) => runFix(appName, fix),
    onSettled: refetchEligibility,
  })
  const wake = useMutation<AppSleep, ApiError, void>({
    mutationFn: () => request(appName, 'POST', '/wake'),
    onSuccess,
  })
  const sleepNow = useMutation<AppSleep, ApiError, void>({
    mutationFn: () => request(appName, 'POST', '/now'),
    onSuccess,
  })
  return { setIdle, wake, sleepNow, applyFix }
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
