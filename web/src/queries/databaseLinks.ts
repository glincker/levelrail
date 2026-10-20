import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { databaseKeys } from './databases'

export interface DatabaseLink {
  app: string
  env_var: string
  field: string
}

export const databaseLinkKeys = {
  list: (database: string) =>
    [...databaseKeys.detail(database), 'links'] as const,
}

function linksUrl(database: string): string {
  return `/api/v1/databases/${encodeURIComponent(database)}/links`
}

export function databaseLinksQueryOptions(database: string) {
  return queryOptions({
    queryKey: databaseLinkKeys.list(database),
    queryFn: async (): Promise<DatabaseLink[]> => {
      const res = await fetch(linksUrl(database))
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `list links failed: ${res.status}`),
        )
      }
      const body = (await res.json()) as { links: DatabaseLink[] }
      return body.links
    },
    retry: false,
  })
}

export function useDatabaseLinks(database: string) {
  return useQuery(databaseLinksQueryOptions(database))
}

export function useConnectDatabaseApp(database: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (req: {
      app: string
      envVar?: string
      field?: string
    }): Promise<DatabaseLink> => {
      const res = await fetch(linksUrl(database), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          app: req.app,
          env_var: req.envVar,
          field: req.field,
        }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `connect app failed: ${res.status}`),
        )
      }
      const out = (await res.json()) as { env_var: string; field: string }
      return { app: req.app, env_var: out.env_var, field: out.field }
    },
    onSuccess: async (link) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: databaseLinkKeys.list(database),
        }),
        queryClient.invalidateQueries({ queryKey: appKeys.detail(link.app) }),
        queryClient.invalidateQueries({ queryKey: appKeys.all }),
      ])
    },
  })
}
