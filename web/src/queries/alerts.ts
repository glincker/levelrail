// Query-key factory and fetchers for GET/POST /api/v1/apps/{name}/alerts
// and DELETE /api/v1/apps/{name}/alerts/{id} (internal/api/alerts.go,
// TASKS.md 2.5/2.7). Kept in its own module for the same reason
// queries/metrics.ts and queries/logs.ts are: a genuinely different
// resource shape than AppDetail, nested under the same app name.
//
// No suspense/loader-primed query here (unlike appDetailQueryOptions):
// AlertRulesPanel is a plain section on the app detail page, not
// something the route loader needs warm before first paint, so a plain
// useQuery with its own loading/error rendering is the right fit, same
// reasoning queries/metrics.ts's header comment already gives for
// MetricsDashboard.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { AlertRule, CreateAlertRuleRequest } from '../types/alerts'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const alertRuleKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'alerts'] as const,
  list: (appName: string) => [...alertRuleKeys.all(appName), 'list'] as const,
}

// GET /api/v1/apps/{name}/alerts. Returns every rule scoped to this
// app, both kinds, enabled and disabled (handleListAlertRules's own doc
// comment), so this fetcher does no client-side filtering either; that
// is AlertRulesPanel's job to render, not this module's job to hide.
export async function fetchAlertRules(appName: string): Promise<AlertRule[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/alerts`)
  if (res.status === 501) {
    throw new ApiError(501, 'alerting is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch alert rules failed: ${res.status}`),
    )
  }
  return (await res.json()) as AlertRule[]
}

export function alertRuleListQueryOptions(appName: string) {
  return queryOptions({
    queryKey: alertRuleKeys.list(appName),
    queryFn: () => fetchAlertRules(appName),
  })
}

export function useAlertRules(appName: string) {
  return useQuery(alertRuleListQueryOptions(appName))
}

// POST /api/v1/apps/{name}/alerts (internal/api/alerts.go's
// handleCreateAlertRule). Returns the full server-assigned rule
// (id, resource_id, zeroed evaluation state), invalidated back into the
// list query on success rather than optimistically appended, since the
// server is the only source of truth for the assigned id.
export async function createAlertRule(
  appName: string,
  req: CreateAlertRuleRequest,
): Promise<AlertRule> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/alerts`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create alert rule failed: ${res.status}`),
    )
  }
  return (await res.json()) as AlertRule
}

export function useCreateAlertRule(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateAlertRuleRequest) => createAlertRule(appName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: alertRuleKeys.list(appName),
      })
    },
  })
}

// DELETE /api/v1/apps/{name}/alerts/{id} (internal/api/alerts.go's
// handleDeleteAlertRule). 204 on success, no body to parse.
export async function deleteAlertRule(
  appName: string,
  id: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/alerts/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete alert rule failed: ${res.status}`),
    )
  }
}

export function useDeleteAlertRule(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteAlertRule(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: alertRuleKeys.list(appName),
      })
    },
  })
}
