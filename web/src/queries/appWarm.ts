// GET/PUT /api/v1/apps/{name}/warm (internal/api/node_pools.go): which nodes
// or label groups keep an app's image pulled ahead of a move.

import { useQuery } from '@tanstack/react-query'
import { useApiMutation } from './useApiMutation'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { NodeLabels } from '../types/nodeMoves'

export interface WarmTargets {
  nodes: string[]
  labels: NodeLabels
}

export const appWarmKeys = {
  detail: (name: string) => ['apps', name, 'warm'] as const,
}

function warmUrl(name: string) {
  return `/api/v1/apps/${encodeURIComponent(name)}/warm`
}

async function parse(res: Response, what: string): Promise<WarmTargets> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as Partial<WarmTargets>
  return { nodes: body.nodes ?? [], labels: body.labels ?? {} }
}

export function useAppWarm(name: string) {
  return useQuery({
    queryKey: appWarmKeys.detail(name),
    queryFn: async () =>
      parse(await fetch(warmUrl(name)), 'fetch warm targets'),
    retry: false,
  })
}

export function useSetAppWarm(name: string) {
  return useApiMutation<WarmTargets, WarmTargets>({
    meta: { errorToast: false },
    fn: async (targets) =>
      parse(
        await fetch(warmUrl(name), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(targets),
        }),
        'set warm targets',
      ),
    invalidate: [appWarmKeys.detail(name)],
  })
}
