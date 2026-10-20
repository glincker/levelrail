import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/api's pendingJoinTokenResource.
export interface PendingJoinToken {
  id: string
  purpose: string
  node_id?: string
  created_at: string
  expires_at: string
}

const URL = '/api/v1/nodes/join-tokens'
const KEY = ['nodes', 'join-tokens'] as const

export async function fetchPendingJoinTokens(
  signal?: AbortSignal,
): Promise<PendingJoinToken[]> {
  const res = await fetch(URL, { signal, cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch join tokens failed: ${res.status}`),
    )
  }
  return (await res.json()) as PendingJoinToken[]
}

export function usePendingJoinTokens() {
  return useQuery({
    queryKey: KEY,
    queryFn: ({ signal }) => fetchPendingJoinTokens(signal),
    retry: false,
    refetchInterval: 30_000,
  })
}

// id undefined revokes every pending token.
export function useRevokeJoinTokens() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id?: string): Promise<{ revoked: number }> => {
      const res = await fetch(id ? `${URL}/${encodeURIComponent(id)}` : URL, {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `revoke join tokens failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as { revoked: number }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}
