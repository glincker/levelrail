import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DatabasePoolerResource } from '../types/databasePooler'

export const databasePoolerKey = (name: string) =>
  ['databases', 'detail', name, 'pooler'] as const

export interface PoolerChange {
  enabled: boolean
  poolSize?: number
  maxClientConn?: number
}

async function poolerRequest(
  name: string,
  method: 'GET' | 'POST' | 'DELETE',
  body?: { pool_size: number; max_client_conn: number },
): Promise<DatabasePoolerResource> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/pooler`,
    body
      ? {
          method,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        }
      : { method },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `pooler request failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabasePoolerResource
}

// Polls while enabled so PoolerReady flips to running without a manual refresh.
export const databasePoolerQuery = (name: string) =>
  queryOptions({
    queryKey: databasePoolerKey(name),
    queryFn: () => poolerRequest(name, 'GET'),
    refetchInterval: (query) =>
      query.state.data?.enabled && query.state.data.status !== 'running'
        ? 3000
        : false,
  })

export function useSetDatabasePooler(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (change: PoolerChange) =>
      change.enabled
        ? poolerRequest(name, 'POST', {
            pool_size: change.poolSize ?? 0,
            max_client_conn: change.maxClientConn ?? 0,
          })
        : poolerRequest(name, 'DELETE'),
    onSuccess: (result) => {
      queryClient.setQueryData(databasePoolerKey(name), result)
      void queryClient.invalidateQueries({
        queryKey: ['databases', 'detail', name],
      })
    },
  })
}
