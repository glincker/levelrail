// Node pools: GET /api/v1/node-pools, PUT/DELETE /api/v1/node-pools/{name}
// and PUT /api/v1/apps/{name}/pool (internal/api/node_pools.go).
import { useQuery } from '@tanstack/react-query'
import { useApiMutation } from './useApiMutation'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface NodePool {
  name: string
  selector: Record<string, string>
}

export const nodePoolKeys = {
  all: ['node-pools'] as const,
}

async function ensureOk(res: Response, what: string): Promise<Response> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return res
}

export function useNodePools() {
  return useQuery({
    queryKey: nodePoolKeys.all,
    queryFn: async ({ signal }) => {
      const res = await ensureOk(
        await fetch('/api/v1/node-pools', { signal }),
        'fetch node pools',
      )
      const body = (await res.json()) as { pools: NodePool[] | null }
      return body.pools ?? []
    },
    retry: false,
  })
}

export function useSetNodePool() {
  return useApiMutation({
    meta: { errorToast: false },
    fn: async (pool: NodePool) => {
      await ensureOk(
        await fetch(`/api/v1/node-pools/${encodeURIComponent(pool.name)}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ selector: pool.selector }),
        }),
        'save node pool',
      )
    },
    invalidate: [nodePoolKeys.all],
  })
}

export function useDeleteNodePool() {
  return useApiMutation({
    meta: { errorToast: false },
    fn: async (name: string) => {
      await ensureOk(
        await fetch(`/api/v1/node-pools/${encodeURIComponent(name)}`, {
          method: 'DELETE',
        }),
        'delete node pool',
      )
    },
    invalidate: [nodePoolKeys.all],
  })
}

// An empty pool name clears the app's pool.
export function useSetAppPool(app: string) {
  return useApiMutation({
    meta: { errorToast: false },
    fn: async (pool: string) => {
      await ensureOk(
        await fetch(`/api/v1/apps/${encodeURIComponent(app)}/pool`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ pool }),
        }),
        'set app pool',
      )
    },
    invalidate: [['placement']],
  })
}
