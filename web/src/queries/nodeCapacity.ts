import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  CleanupResult,
  CleanupRun,
  NodeCapacity,
  NodeGuardrailSettings,
} from '../types/nodeCapacity'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { nodeKeys } from './nodes'

const REFRESH_MS = 30_000
const JSON_HEADERS = { 'Content-Type': 'application/json' }

export const nodeCapacityKeys = {
  capacity: (id: string) => [...nodeKeys.detail(id), 'capacity'] as const,
  runs: (id: string) => [...nodeKeys.detail(id), 'cleanup-runs'] as const,
}

function path(id: string, tail: string): string {
  return `/api/v1/nodes/${encodeURIComponent(id)}/${tail}`
}

async function parse<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useNodeCapacity(id: string) {
  return useQuery({
    queryKey: nodeCapacityKeys.capacity(id),
    queryFn: async () =>
      parse<NodeCapacity>(await fetch(path(id, 'capacity')), 'node capacity'),
    retry: false,
    refetchInterval: REFRESH_MS,
  })
}

export function useNodeCleanupRuns(id: string) {
  return useQuery({
    queryKey: nodeCapacityKeys.runs(id),
    queryFn: async () =>
      parse<CleanupRun[]>(
        await fetch(path(id, 'cleanup-runs')),
        'cleanup history',
      ),
    retry: false,
  })
}

export function useCleanupPreview(id: string) {
  return useMutation({
    mutationFn: async () =>
      parse<CleanupResult>(
        await fetch(path(id, 'cleanup'), {
          method: 'POST',
          headers: JSON_HEADERS,
          body: JSON.stringify({ dry_run: true }),
        }),
        'cleanup preview',
      ),
  })
}

export function useRunCleanup(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () =>
      parse<CleanupResult>(
        await fetch(path(id, 'cleanup'), {
          method: 'POST',
          headers: JSON_HEADERS,
          body: JSON.stringify({ dry_run: false, confirm: true }),
        }),
        'cleanup',
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: nodeCapacityKeys.capacity(id) })
      void qc.invalidateQueries({ queryKey: nodeCapacityKeys.runs(id) })
    },
  })
}

export function useSaveGuardrails(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (settings: NodeGuardrailSettings) =>
      parse<NodeCapacity>(
        await fetch(path(id, 'guardrails'), {
          method: 'PUT',
          headers: JSON_HEADERS,
          body: JSON.stringify(settings),
        }),
        'save guardrails',
      ),
    onSuccess: (data) => {
      qc.setQueryData(nodeCapacityKeys.capacity(id), data)
    },
  })
}
