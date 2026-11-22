// POST /api/v1/builds/cache/prune with an explicit idle age; without apply it
// only previews what would be freed.
import { useApiMutation } from './useApiMutation'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { BuildCachePrune } from './buildCenter'

export interface NodeCachePruneArgs {
  hours: number
  apply: boolean
}

export function useNodeCachePrune(node: string) {
  return useApiMutation({
    meta: { errorToast: false },
    fn: async ({
      hours,
      apply,
    }: NodeCachePruneArgs): Promise<BuildCachePrune> => {
      const res = await fetch('/api/v1/builds/cache/prune', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ node, older_than_hours: hours, apply }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `prune build cache failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as BuildCachePrune
    },
    invalidate: (vars) => (vars.apply ? [['builds']] : []),
  })
}
