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
      await readErrorMessage(res, `fetch domain maintenance failed: ${res.status}`),
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
): Promise<DomainMaintenance> {
  const res = await fetch(maintenancePath(appName, domain), { method: 'PUT' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `enable domain maintenance failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainMaintenance
}

export function useSetDomainMaintenance(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainMaintenance, ApiError, void>({
    mutationFn: () => setDomainMaintenance(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainMaintenanceKeys.detail(appName, domain), updated)
    },
  })
}

async function clearDomainMaintenance(
  appName: string,
  domain: string,
): Promise<DomainMaintenance> {
  const res = await fetch(maintenancePath(appName, domain), { method: 'DELETE' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `disable domain maintenance failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainMaintenance
}

export function useClearDomainMaintenance(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainMaintenance, ApiError, void>({
    mutationFn: () => clearDomainMaintenance(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(domainMaintenanceKeys.detail(appName, domain), updated)
    },
  })
}
