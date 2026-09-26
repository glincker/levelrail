import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'

export type BulkAction =
  | 'redeploy'
  | 'restart'
  | 'stop'
  | 'start'
  | 'add-tag'
  | 'remove-tag'
  | 'set-environment'
  | 'move-to-project'
  | 'delete'

export interface BulkRequest {
  action: BulkAction
  names: string[]
  value?: string
  dry_run?: boolean
  confirm_names?: string[]
}

export interface BulkAppResult {
  name: string
  status: 'ok' | 'would_apply' | 'denied' | 'not_found' | 'skipped' | 'error'
  message?: string
}

export interface BulkResponse {
  action: BulkAction
  dry_run: boolean
  results: BulkAppResult[]
  counts: Record<string, number>
}

export interface AppsSummary {
  total: number
  running: number
  failing: number
  deploying: number
  stopped: number
  unknown: number
}

export function appsSummaryQueryOptions() {
  return queryOptions({
    queryKey: [...appKeys.all, 'summary'] as const,
    queryFn: async (): Promise<AppsSummary> => {
      const res = await fetch('/api/v1/apps-summary')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch apps summary failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as AppsSummary
    },
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
}

export async function postBulk(req: BulkRequest): Promise<BulkResponse> {
  const res = await fetch('/api/v1/apps/bulk', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `bulk ${req.action} failed: ${res.status}`),
    )
  }
  return (await res.json()) as BulkResponse
}

export function useBulkApps() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: postBulk,
    onSuccess: (data) => {
      if (!data.dry_run) {
        void queryClient.invalidateQueries({ queryKey: appKeys.all })
      }
    },
  })
}
