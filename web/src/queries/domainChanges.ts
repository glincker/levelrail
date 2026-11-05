// Recent DNS changes for a domain and the one-click rollback.
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DomainChange {
  run_id: string
  kind: string
  created_at: string
  summary: string
  rollback_until: string
  can_rollback: boolean
  already_undone: boolean
}

export interface RollbackRecordResult {
  name: string
  type: string
  outcome: string
  message?: string
}

export interface RollbackResult {
  results: RollbackRecordResult[]
  summary?: string
}

export const domainChangesKeys = {
  list: (domain: string) => ['domains', domain, 'changes'] as const,
}

export function domainChangesQueryOptions(domain: string) {
  return queryOptions({
    queryKey: domainChangesKeys.list(domain),
    queryFn: async (): Promise<DomainChange[]> => {
      const res = await fetch(
        `/api/v1/domains/${encodeURIComponent(domain)}/changes`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `changes failed: ${res.status}`),
        )
      }
      const body = (await res.json()) as { changes?: DomainChange[] }
      return body.changes ?? []
    },
  })
}

export function useRollbackDomainChange(domain: string) {
  const queryClient = useQueryClient()
  return useMutation<RollbackResult, ApiError, string>({
    mutationFn: async (runId) => {
      const res = await fetch(
        `/api/v1/domains/${encodeURIComponent(domain)}/changes/${encodeURIComponent(runId)}/rollback`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `rollback failed: ${res.status}`),
        )
      }
      return (await res.json()) as RollbackResult
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: domainChangesKeys.list(domain),
      })
      // Domain-scoped DNS reads: records, checks and the setup plan.
      void queryClient.invalidateQueries({
        predicate: (q) =>
          q.queryKey.includes(domain) &&
          (q.queryKey.includes('dns-records') ||
            q.queryKey.includes('check') ||
            q.queryKey.includes('dns-setup') ||
            q.queryKey.includes('cutover')),
      })
      void queryClient.invalidateQueries({ queryKey: ['dns-setup'] })
    },
  })
}
