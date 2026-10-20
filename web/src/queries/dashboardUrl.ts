// GET/PUT /api/v1/settings/dashboard-url (internal/api/dashboard_url.go).
import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DashboardUrlSetting {
  dashboard_url: string
}

const dashboardUrlKey = ['settings', 'dashboard-url'] as const

export async function fetchDashboardUrl(): Promise<DashboardUrlSetting> {
  const res = await fetch('/api/v1/settings/dashboard-url')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch dashboard url failed: ${res.status}`),
    )
  }
  return (await res.json()) as DashboardUrlSetting
}

export async function updateDashboardUrl(
  setting: DashboardUrlSetting,
): Promise<DashboardUrlSetting> {
  const res = await fetch('/api/v1/settings/dashboard-url', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(setting),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update dashboard url failed: ${res.status}`),
    )
  }
  return (await res.json()) as DashboardUrlSetting
}

export function dashboardUrlQueryOptions() {
  return queryOptions({
    queryKey: dashboardUrlKey,
    queryFn: fetchDashboardUrl,
    staleTime: 60_000,
  })
}

export function useDashboardUrl() {
  return useSuspenseQuery(dashboardUrlQueryOptions())
}

export function useUpdateDashboardUrl() {
  const queryClient = useQueryClient()
  return useMutation<DashboardUrlSetting, ApiError, DashboardUrlSetting>({
    mutationFn: updateDashboardUrl,
    onSuccess: (updated) => {
      queryClient.setQueryData(dashboardUrlKey, updated)
    },
  })
}
