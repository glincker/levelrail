import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CustomDashboard,
  DashboardDoc,
  DashboardTemplate,
  DashboardWrite,
} from '../types/dashboards'

export const dashboardKeys = {
  all: ['custom-dashboards'] as const,
  list: () => [...dashboardKeys.all, 'list'] as const,
  templates: () => [...dashboardKeys.all, 'templates'] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function jsonInit(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function dashboardListQueryOptions() {
  return queryOptions({
    queryKey: dashboardKeys.list(),
    queryFn: ({ signal }) =>
      request<CustomDashboard[]>(
        '/api/v1/dashboards',
        { signal },
        'list dashboards',
      ),
    staleTime: 15_000,
  })
}

export function useDashboards() {
  return useQuery(dashboardListQueryOptions())
}

export function useDashboardTemplates() {
  return useQuery({
    queryKey: dashboardKeys.templates(),
    queryFn: ({ signal }) =>
      request<DashboardTemplate[]>(
        '/api/v1/dashboards/templates',
        { signal },
        'load dashboard templates',
      ),
    staleTime: Infinity,
  })
}

export function useCreateDashboard() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: DashboardWrite) =>
      request<CustomDashboard>(
        '/api/v1/dashboards',
        jsonInit('POST', body),
        'create dashboard',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: dashboardKeys.list() }),
  })
}

export function useUpdateDashboard() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: DashboardWrite }) =>
      request<CustomDashboard>(
        `/api/v1/dashboards/${encodeURIComponent(id)}`,
        jsonInit('PUT', body),
        'save dashboard',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: dashboardKeys.list() }),
  })
}

export function useDeleteDashboard() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `/api/v1/dashboards/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'delete dashboard',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: dashboardKeys.list() }),
  })
}

export function useImportDashboard() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (doc: DashboardDoc) =>
      request<CustomDashboard>(
        '/api/v1/dashboards/import',
        jsonInit('POST', doc),
        'import dashboard',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: dashboardKeys.list() }),
  })
}
