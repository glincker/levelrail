// Read-only queries for the integrations hub (internal/api connections read model).

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CatalogEntry,
  Connection,
  ConnectionCatalogResponse,
  ConnectionListResponse,
} from '../types/connections'

export const connectionKeys = {
  all: ['connections'] as const,
  list: () => [...connectionKeys.all, 'list'] as const,
  catalog: () => [...connectionKeys.all, 'catalog'] as const,
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

// A null or missing array is an empty list, never a crash.
export async function fetchConnections(): Promise<Connection[]> {
  const body = await getJson<ConnectionListResponse | null>(
    '/api/v1/connections',
    'fetch connections',
  )
  return body?.connections ?? []
}

export async function fetchConnectionCatalog(): Promise<CatalogEntry[]> {
  const body = await getJson<ConnectionCatalogResponse | null>(
    '/api/v1/connections/catalog',
    'fetch integration catalog',
  )
  return body?.catalog ?? []
}

export function connectionsQueryOptions() {
  return queryOptions({
    queryKey: connectionKeys.list(),
    queryFn: fetchConnections,
  })
}

export function connectionCatalogQueryOptions() {
  return queryOptions({
    queryKey: connectionKeys.catalog(),
    queryFn: fetchConnectionCatalog,
  })
}

export function useConnections() {
  return useQuery(connectionsQueryOptions())
}

export function useConnectionCatalog() {
  return useQuery(connectionCatalogQueryOptions())
}
