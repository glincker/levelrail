// Query-key factory, fetchers, and mutations for a database's own init
// scripts. Mirrors queries/cloneRestore.ts's own list-plus-mutation
// shape.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  DatabaseInitScript,
  SetDatabaseInitScriptRequest,
} from '../types/databaseInitScript'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const databaseInitScriptKeys = {
  list: (databaseName: string) =>
    ['databases', databaseName, 'init-scripts'] as const,
}

export async function fetchDatabaseInitScripts(
  databaseName: string,
): Promise<DatabaseInitScript[]> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/init-scripts`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch init scripts failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as DatabaseInitScript[] | null
  return body ?? []
}

export function databaseInitScriptsQueryOptions(databaseName: string) {
  return queryOptions({
    queryKey: databaseInitScriptKeys.list(databaseName),
    queryFn: () => fetchDatabaseInitScripts(databaseName),
  })
}

export function useDatabaseInitScripts(databaseName: string) {
  return useQuery(databaseInitScriptsQueryOptions(databaseName))
}

async function parseInitScriptResponse(
  res: Response,
  action: string,
): Promise<DatabaseInitScript> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${action} init script failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseInitScript
}

export async function createDatabaseInitScript(
  databaseName: string,
  req: SetDatabaseInitScriptRequest,
): Promise<DatabaseInitScript> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/init-scripts`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  return parseInitScriptResponse(res, 'create')
}

export async function updateDatabaseInitScript(
  databaseName: string,
  id: string,
  req: SetDatabaseInitScriptRequest,
): Promise<DatabaseInitScript> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/init-scripts/${encodeURIComponent(id)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  return parseInitScriptResponse(res, 'update')
}

export async function deleteDatabaseInitScript(
  databaseName: string,
  id: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/init-scripts/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok && res.status !== 204) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete init script failed: ${res.status}`),
    )
  }
}

export function useCreateDatabaseInitScript(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<DatabaseInitScript, ApiError, SetDatabaseInitScriptRequest>({
    mutationFn: (req) => createDatabaseInitScript(databaseName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: databaseInitScriptKeys.list(databaseName),
      })
    },
  })
}

export function useUpdateDatabaseInitScript(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<
    DatabaseInitScript,
    ApiError,
    { id: string; req: SetDatabaseInitScriptRequest }
  >({
    mutationFn: ({ id, req }) => updateDatabaseInitScript(databaseName, id, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: databaseInitScriptKeys.list(databaseName),
      })
    },
  })
}

export function useDeleteDatabaseInitScript(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) => deleteDatabaseInitScript(databaseName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: databaseInitScriptKeys.list(databaseName),
      })
    },
  })
}
