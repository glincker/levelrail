// Query-key factory and fetcher for GET/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/maintenance
// (internal/api/domain_maintenance.go's domainMaintenanceResource):
// maintenance mode for one of an app's domains, enforced by the
// embedded Caddy ingress on the next reconcile pass. Mirrors
// queries/domainBasicAuth.ts's shape, minus the secrets dependency:
// there is no credential involved, so no 501 "not configured" case
// either.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's domainMaintenanceResource wire shape exactly.
export interface DomainMaintenance {
  domain: string
  enabled: boolean
  title?: string
  message?: string
  retry_after_seconds?: number
  // until is RFC 3339; set when a linked scheduled window ends it.
  until?: string
  window_id?: string
  bypass?: string[]
  bypass_health_check?: boolean
  started_at?: string
  // auto_off_at is the safety cut-off for open ended maintenance.
  auto_off_at?: string
  // ongoing: the public status page lists this as ongoing (no end time).
  ongoing?: boolean
  // alerts_silenced: a maintenance silence mutes the app's probe alerts.
  alerts_silenced?: boolean
  silence_id?: string
}

// SetDomainMaintenanceRequest is PUT's optional body; omit it for defaults.
export interface SetDomainMaintenanceRequest {
  title?: string
  message?: string
  retry_after_seconds?: number
  until?: string
  bypass?: string[]
  bypass_health_check?: boolean
  // max_duration_seconds omitted takes the server default, 0 means no limit.
  max_duration_seconds?: number
}

export interface FleetMaintenanceEntry extends DomainMaintenance {
  app: string
  elapsed_seconds: number
}

export interface FleetMaintenance {
  items: FleetMaintenanceEntry[]
  max_duration_seconds: number
  attention_after_seconds: number
}

export interface MaintenancePreview {
  html: string
  status: number
  retry_after: string
}

export const fleetMaintenanceKey = ['maintenance', 'domains'] as const

export function fleetMaintenanceQueryOptions() {
  return queryOptions({
    queryKey: fleetMaintenanceKey,
    queryFn: async (): Promise<FleetMaintenance> => {
      const res = await fetch('/api/v1/maintenance/domains')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `list maintenance failed: ${res.status}`),
        )
      }
      return (await res.json()) as FleetMaintenance
    },
    staleTime: 15_000,
    refetchInterval: 60_000,
  })
}

export function useFleetMaintenance() {
  return useQuery(fleetMaintenanceQueryOptions())
}

// The server renders the page with the ingress's own template, so the
// preview is what visitors get, not a copy that can drift.
export function useMaintenancePreview(
  title: string,
  message: string,
  retryAfterSeconds: number,
) {
  return useQuery({
    queryKey: ['maintenance', 'preview', title, message, retryAfterSeconds],
    queryFn: async (): Promise<MaintenancePreview> => {
      const res = await fetch('/api/v1/maintenance/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title,
          message,
          retry_after_seconds: retryAfterSeconds,
        }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `preview failed: ${res.status}`),
        )
      }
      return (await res.json()) as MaintenancePreview
    },
    staleTime: 60_000,
    placeholderData: (prev) => prev,
  })
}

export const domainMaintenanceKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'maintenance'] as const,
}

function maintenancePath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/maintenance`
}

export async function fetchDomainMaintenance(
  appName: string,
  domain: string,
): Promise<DomainMaintenance> {
  const res = await fetch(maintenancePath(appName, domain))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch domain maintenance failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainMaintenance
}

export function domainMaintenanceQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainMaintenanceKeys.detail(appName, domain),
    queryFn: () => fetchDomainMaintenance(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainMaintenance(appName: string, domain: string) {
  return useQuery(domainMaintenanceQueryOptions(appName, domain))
}

async function setDomainMaintenance(
  appName: string,
  domain: string,
  body?: SetDomainMaintenanceRequest,
): Promise<DomainMaintenance> {
  const res = await fetch(
    maintenancePath(appName, domain),
    body
      ? {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        }
      : { method: 'PUT' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `enable domain maintenance failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainMaintenance
}

export function useSetDomainMaintenance(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<
    DomainMaintenance,
    ApiError,
    SetDomainMaintenanceRequest | undefined
  >({
    mutationFn: (body) => setDomainMaintenance(appName, domain, body),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainMaintenanceKeys.detail(appName, domain),
        updated,
      )
      void queryClient.invalidateQueries({ queryKey: fleetMaintenanceKey })
    },
  })
}

async function clearDomainMaintenance(
  appName: string,
  domain: string,
): Promise<DomainMaintenance> {
  const res = await fetch(maintenancePath(appName, domain), {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `disable domain maintenance failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DomainMaintenance
}

export function useClearDomainMaintenance(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainMaintenance, ApiError, void>({
    mutationFn: () => clearDomainMaintenance(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainMaintenanceKeys.detail(appName, domain),
        updated,
      )
      void queryClient.invalidateQueries({ queryKey: fleetMaintenanceKey })
    },
  })
}
