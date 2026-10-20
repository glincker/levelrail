// GET/PUT /api/v1/apps/{name}/schedule and
// GET /api/v1/apps/{name}/schedule/history (internal/api/app_schedule.go):
// a per-app recurring redeploy of a branch's latest commit.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AppSchedule {
  service_name: string
  cron: string
  branch: string
  timezone: string
  enabled: boolean
  next_run_at?: string
}

export interface AppScheduleInput {
  cron: string
  branch: string
  timezone?: string
  enabled?: boolean
}

export interface AppScheduleHistoryEntry {
  id: string
  scheduled_for: string
  fired_at: string
  status: 'fired' | 'skipped_freeze' | 'failed'
  reason?: string
}

export const appScheduleKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'schedule'] as const,
  history: (appName: string) =>
    [...appKeys.detail(appName), 'schedule', 'history'] as const,
}

function scheduleURL(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}/schedule`
}

async function fetchAppSchedule(appName: string): Promise<AppSchedule | null> {
  const res = await fetch(scheduleURL(appName))
  if (res.status === 404) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app schedule failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppSchedule
}

export function appScheduleQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appScheduleKeys.detail(appName),
    queryFn: () => fetchAppSchedule(appName),
    retry: false,
  })
}

export function useAppSchedule(appName: string) {
  return useQuery(appScheduleQueryOptions(appName))
}

async function putAppSchedule(
  appName: string,
  input: AppScheduleInput,
): Promise<AppSchedule> {
  const res = await fetch(scheduleURL(appName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `save app schedule failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppSchedule
}

export function useSetAppSchedule(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<AppSchedule, ApiError, AppScheduleInput>({
    mutationFn: (input) => putAppSchedule(appName, input),
    onSuccess: (data) => {
      queryClient.setQueryData(appScheduleKeys.detail(appName), data)
      void queryClient.invalidateQueries({
        queryKey: appScheduleKeys.history(appName),
      })
    },
  })
}

async function fetchAppScheduleHistory(
  appName: string,
): Promise<AppScheduleHistoryEntry[]> {
  const res = await fetch(`${scheduleURL(appName)}/history?limit=10`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch app schedule history failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AppScheduleHistoryEntry[]
}

export function useAppScheduleHistory(appName: string, enabled: boolean) {
  return useQuery({
    queryKey: appScheduleKeys.history(appName),
    queryFn: () => fetchAppScheduleHistory(appName),
    enabled,
    retry: false,
  })
}
