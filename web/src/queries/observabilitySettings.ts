// GET/PUT /api/v1/settings/observability
// (internal/api/observability_settings.go's observabilitySettingsResource).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const observabilitySettingsKeys = {
  all: ['observability-settings'] as const,
}

// ObservabilitySettings mirrors observabilitySettingsResource exactly.
// remote_read_path is server-generated (the real address
// handlePrometheusRead serves), never sent on PUT.
export interface ObservabilitySettings {
  external_dashboard_url: string
  remote_read_path: string
}

export async function fetchObservabilitySettings(): Promise<ObservabilitySettings> {
  const res = await fetch('/api/v1/settings/observability')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch observability settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ObservabilitySettings
}

export function observabilitySettingsQueryOptions() {
  return queryOptions({
    queryKey: observabilitySettingsKeys.all,
    queryFn: fetchObservabilitySettings,
    staleTime: 60_000,
  })
}

export function useObservabilitySettings() {
  return useSuspenseQuery(observabilitySettingsQueryOptions())
}

// Plain useQuery, not suspense: this backs ViewInGrafanaLink, a small
// addition rendered inline on the app/node metrics dashboards, the same
// "must never take down the page it lives in" shape useActivityEvents
// (queries/activity.ts) already establishes for an optional signal.
// retry: false so a real outage doesn't hammer this on every chart render.
export function useObservabilitySettingsOptional() {
  return useQuery({ ...observabilitySettingsQueryOptions(), retry: false })
}

export async function updateObservabilitySettings(
  settings: Pick<ObservabilitySettings, 'external_dashboard_url'>,
): Promise<ObservabilitySettings> {
  const res = await fetch('/api/v1/settings/observability', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update observability settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ObservabilitySettings
}

export function useUpdateObservabilitySettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateObservabilitySettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(observabilitySettingsKeys.all, updated)
    },
  })
}
