import { useMutation, useQueryClient } from '@tanstack/react-query'
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
  | 'scale'
  | 'rollback'

export interface BulkRequest {
  action: BulkAction
  names: string[]
  value?: string
  dry_run?: boolean
  confirm_names?: string[]
  confirm_count?: number
  replicas?: number
  min_replicas?: number
  max_replicas?: number
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
    meta: { errorToast: false },
    mutationFn: postBulk,
    onSuccess: (data) => {
      if (!data.dry_run) {
        void queryClient.invalidateQueries({ queryKey: appKeys.all })
      }
    },
  })
}

export type ScaleFields = Pick<
  BulkRequest,
  'replicas' | 'min_replicas' | 'max_replicas'
>

/** Reads "3" as a replica count and "2-6" as autoscale bounds; null when neither fits. */
export function parseScaleValue(raw: string): ScaleFields | null {
  const text = raw.trim()
  const range = /^(\d+)\s*-\s*(\d+)$/.exec(text)
  if (range) {
    const min = Number(range[1])
    const max = Number(range[2])
    return min >= 1 && max >= min
      ? { min_replicas: min, max_replicas: max }
      : null
  }
  if (/^\d+$/.test(text) && Number(text) >= 1) return { replicas: Number(text) }
  return null
}
