import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface OrphanFinding {
  kind: 'container' | 'volume' | 'certificate'
  name: string
  node_id: string
  reason: string
  placed_on?: string
  skip?: string
  first_seen_at?: string
  reap_after?: string
  due: boolean
}

export interface OrphanReport {
  dry_run: boolean
  findings: OrphanFinding[]
  removed: { kind: string; name: string }[]
  failed?: { kind: string; name: string; error: string }[]
  unreachable?: string[]
  halted?: string
}

export const orphanKeys = { all: ['orphans'] as const }

async function fetchOrphans(): Promise<OrphanReport | null> {
  const res = await fetch('/api/v1/system/orphans')
  if (res.status === 501) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list orphans failed: ${res.status}`),
    )
  }
  return (await res.json()) as OrphanReport
}

export function useOrphans() {
  return useQuery({ queryKey: orphanKeys.all, queryFn: fetchOrphans })
}

async function reapOrphans(dryRun: boolean): Promise<OrphanReport> {
  const res = await fetch(
    `/api/v1/system/orphans/reap${dryRun ? '?dry_run=true' : ''}`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `reap orphans failed: ${res.status}`),
    )
  }
  return (await res.json()) as OrphanReport
}

export function useReapOrphans() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: reapOrphans,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: orphanKeys.all })
    },
  })
}
