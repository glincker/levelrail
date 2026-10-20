// Recorded control plane version transitions for Settings > Updates
// (internal/api/updates_history.go).

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { updatesKeys } from './updates'

export type UpgradeKind =
  | 'installed'
  | 'adopted'
  | 'upgraded'
  | 'rolled_back'
  | 'rebuilt'
  | 'development'
  | 'changed'

export type NotesState = 'pending' | 'fetched' | 'unavailable'

export interface UpgradeHistoryItem {
  id: string
  kind: UpgradeKind
  from_version: string
  to_version: string
  channel: string
  schema_before: number | null
  schema_after: number | null
  schema_moved: boolean
  occurred_at: string
  initiator: string
  method: string
  backup_name: string
  health: string
  notes: string
  notes_state: NotesState
  release_url: string
  compare_url: string
  acknowledged: boolean
  acked_by: string
  acked_at: string | null
  rollback_available: boolean
}

export interface AgentVersionChange {
  node_id: string
  node_name: string
  from_version: string
  to_version: string
  observed_at: string
}

export interface UpgradeHistory {
  current_version: string
  unacknowledged: number
  entries: UpgradeHistoryItem[]
  agent_changes: AgentVersionChange[]
}

const historyKey = [...updatesKeys.all, 'history'] as const

export function upgradeHistoryQueryOptions() {
  return queryOptions({
    queryKey: historyKey,
    queryFn: async (): Promise<UpgradeHistory> => {
      const res = await fetch('/api/v1/updates/history')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch upgrade history failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as UpgradeHistory
    },
    staleTime: 30_000,
  })
}

export function useAckUpgrade() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<UpgradeHistoryItem> => {
      const res = await fetch(
        `/api/v1/updates/history/${encodeURIComponent(id)}/ack`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `acknowledge failed: ${res.status}`),
        )
      }
      return (await res.json()) as UpgradeHistoryItem
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: historyKey }),
  })
}
