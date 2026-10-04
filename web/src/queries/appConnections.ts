// Query-key factory, fetchers and mutation hooks for one app's
// multi-connection database attachments: GET/POST /api/v1/apps/{name}/connections,
// DELETE .../connections/{env_var}, and GET
// /api/v1/apps/{name}/connectable-databases (internal/api/apps_connections.go).
// The DatabaseEnv-map-backed sibling of queries/apps.ts's own
// useSetAppDatabase/useClearAppDatabase (DatabaseAttachment's single-slot
// shape): an app can have any number of these.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AppConnection {
  env_var: string
  database_name: string
  field: string
  // host is the value this connection resolves to at deploy time: a
  // mesh DNS name (mesh_dns: true) or a plain Docker container name
  // (mesh_dns: false, reachable only when app and database share a
  // node).
  host: string
  mesh_dns: boolean
  node_id?: string
  cross_node: boolean
}

export interface ConnectableDatabase {
  name: string
  engine: string
  node_id?: string
  cross_node: boolean
  already_connected: boolean
  connected_env_vars?: string[]
}

export const appConnectionKeys = {
  all: ['appConnections'] as const,
  list: (appName: string) => [...appConnectionKeys.all, appName] as const,
  connectable: (appName: string) =>
    [...appConnectionKeys.all, appName, 'connectable'] as const,
}

export async function fetchAppConnections(
  appName: string,
): Promise<AppConnection[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/connections`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch app connections failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AppConnection[]
}

export function appConnectionsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appConnectionKeys.list(appName),
    queryFn: () => fetchAppConnections(appName),
    enabled: appName.length > 0,
  })
}

export function useAppConnections(appName: string) {
  return useQuery(appConnectionsQueryOptions(appName))
}

export async function fetchConnectableDatabases(
  appName: string,
): Promise<ConnectableDatabase[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/connectable-databases`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch connectable databases failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ConnectableDatabase[]
}

export function connectableDatabasesQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appConnectionKeys.connectable(appName),
    queryFn: () => fetchConnectableDatabases(appName),
    enabled: appName.length > 0,
  })
}

export function useConnectableDatabases(appName: string) {
  return useQuery(connectableDatabasesQueryOptions(appName))
}

export interface CreateAppConnectionInput {
  database: string
  field?: string
  envVar?: string
}

export async function createAppConnection(
  appName: string,
  input: CreateAppConnectionInput,
): Promise<AppConnection> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/connections`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        database: input.database,
        field: input.field,
        env_var: input.envVar,
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create app connection failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AppConnection
}

// Invalidates both the connections list and the connectable-databases
// list on success: connecting a database changes both (a new entry
// appears in one, already_connected flips in the other), and neither
// response alone carries enough to patch the other's cache correctly.
export function useCreateAppConnection(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAppConnectionInput) =>
      createAppConnection(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appConnectionKeys.list(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: appConnectionKeys.connectable(appName),
      })
    },
  })
}

export async function deleteAppConnection(
  appName: string,
  envVar: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/connections/${encodeURIComponent(envVar)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `delete app connection failed: ${res.status}`),
  )
}

export function useDeleteAppConnection(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (envVar: string) => deleteAppConnection(appName, envVar),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appConnectionKeys.list(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: appConnectionKeys.connectable(appName),
      })
    },
  })
}
