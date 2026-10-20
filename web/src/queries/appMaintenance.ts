// GET/PUT/DELETE /api/v1/apps/{name}/maintenance
// (internal/api/app_maintenance.go): maintenance for every domain of an app,
// applied to all of them or none.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import {
  fleetMaintenanceKey,
  type DomainMaintenance,
  type SetDomainMaintenanceRequest,
} from './domainMaintenance'

export interface AppMaintenanceDomain extends DomainMaintenance {
  elapsed_seconds?: number
}

export interface AppMaintenance {
  app: string
  enabled: boolean
  all: boolean
  total_domains: number
  active_domains: number
  domains: AppMaintenanceDomain[]
}

export const appMaintenanceKey = (app: string) =>
  ['apps', app, 'maintenance'] as const

function path(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/maintenance`
}

async function request(
  app: string,
  init: RequestInit | undefined,
  what: string,
): Promise<AppMaintenance> {
  const res = await fetch(path(app), init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppMaintenance
}

export function appMaintenanceQueryOptions(app: string) {
  return queryOptions({
    queryKey: appMaintenanceKey(app),
    queryFn: () => request(app, undefined, 'fetch app maintenance'),
    staleTime: 30_000,
    refetchInterval: 60_000,
  })
}

export function useAppMaintenance(app: string) {
  return useQuery(appMaintenanceQueryOptions(app))
}

function useRefresh(app: string) {
  const queryClient = useQueryClient()
  return (updated: AppMaintenance) => {
    queryClient.setQueryData(appMaintenanceKey(app), updated)
    void queryClient.invalidateQueries({ queryKey: fleetMaintenanceKey })
    void queryClient.invalidateQueries({ queryKey: ['apps', app, 'domains'] })
    void queryClient.invalidateQueries({ queryKey: ['domains'] })
  }
}

export function useSetAppMaintenance(app: string) {
  const refresh = useRefresh(app)
  return useMutation<
    AppMaintenance,
    ApiError,
    SetDomainMaintenanceRequest | undefined
  >({
    mutationFn: (body) =>
      request(
        app,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body ?? {}),
        },
        'enable app maintenance',
      ),
    onSuccess: refresh,
  })
}

export function useClearAppMaintenance(app: string) {
  const refresh = useRefresh(app)
  return useMutation<AppMaintenance, ApiError, void>({
    mutationFn: () => request(app, { method: 'DELETE' }, 'end app maintenance'),
    onSuccess: refresh,
  })
}
