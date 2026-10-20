// Query-key factory and fetchers for GET /api/v1/integrations and
// GET/POST/DELETE /api/v1/apps/{name}/integrations
// (internal/api/app_integrations.go). Kept in its own module for the
// same reason queries/scheduledTasks.ts is: a genuinely different
// resource shape than AppDetail, nested under the same app name, plus
// one global catalog list that isn't app-scoped at all.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  AppIntegration,
  AttachAppIntegrationRequest,
  IntegrationCatalogEntry,
} from '../types/appIntegrations'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const integrationCatalogKeys = {
  all: ['integration-catalog'] as const,
}

export const appIntegrationKeys = {
  all: (appName: string) =>
    [...appKeys.detail(appName), 'integrations'] as const,
  list: (appName: string) =>
    [...appIntegrationKeys.all(appName), 'list'] as const,
}

export async function fetchIntegrationCatalog(): Promise<
  IntegrationCatalogEntry[]
> {
  const res = await fetch('/api/v1/integrations')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch integration catalog failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as IntegrationCatalogEntry[]
}

export function integrationCatalogQueryOptions() {
  return queryOptions({
    queryKey: integrationCatalogKeys.all,
    queryFn: fetchIntegrationCatalog,
  })
}

export function useIntegrationCatalog() {
  return useQuery(integrationCatalogQueryOptions())
}

export async function fetchAppIntegrations(
  appName: string,
): Promise<AppIntegration[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/integrations`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch app integrations failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AppIntegration[]
}

export function appIntegrationListQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appIntegrationKeys.list(appName),
    queryFn: () => fetchAppIntegrations(appName),
  })
}

export function useAppIntegrations(appName: string) {
  return useQuery(appIntegrationListQueryOptions(appName))
}

export async function attachAppIntegration(
  appName: string,
  req: AttachAppIntegrationRequest,
): Promise<AppIntegration> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/integrations`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `attach integration failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppIntegration
}

export function useAttachAppIntegration(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: AttachAppIntegrationRequest) =>
      attachAppIntegration(appName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appIntegrationKeys.list(appName),
      })
    },
  })
}

export async function detachAppIntegration(
  appName: string,
  id: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/integrations/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `detach integration failed: ${res.status}`),
    )
  }
}

export function useDetachAppIntegration(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => detachAppIntegration(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appIntegrationKeys.list(appName),
      })
    },
  })
}
