// Queries and mutations for the node failure policy (internal/api/node_failure.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { NodeFailureStatus } from '../types/nodeFailure'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { nodeKeys } from './nodes'

const failureKey = (id: string) => [...nodeKeys.detail(id), 'failure'] as const

async function call(
  id: string,
  path: string,
  method: 'GET' | 'PUT' | 'POST',
  body?: unknown,
): Promise<NodeFailureStatus> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(id)}${path}`, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `node failure request failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeFailureStatus
}

export function useNodeFailure(id: string) {
  return useQuery({
    queryKey: failureKey(id),
    queryFn: () => call(id, '/failure', 'GET'),
    refetchInterval: 10_000,
  })
}

function useFailureMutation<V>(
  id: string,
  run: (v: V) => Promise<NodeFailureStatus>,
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: run,
    onSuccess: (status) => {
      queryClient.setQueryData(failureKey(id), status)
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
    },
  })
}

export function useSetNodeFailurePolicy(id: string) {
  return useFailureMutation(id, (v: { mode: string; grace_seconds: number }) =>
    call(id, '/failure-policy', 'PUT', v),
  )
}

export function useMarkNodeLost(id: string) {
  return useFailureMutation(id, (v: { dry_run: boolean }) =>
    call(id, '/failure/lost', 'POST', {
      dry_run: v.dry_run,
      confirm: !v.dry_run,
    }),
  )
}

export function useResolveNodeFailure(id: string) {
  return useFailureMutation(
    id,
    (v: {
      kind: string
      name: string
      action: 'wait' | 'restore'
      target_node_id?: string
    }) => call(id, '/failure/resolve', 'POST', v),
  )
}
