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
  | 'set-memory'
  | 'set-cpu'
  | 'set-env'

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
  memory_bytes?: number
  nano_cpus?: number
  env_key?: string
  env_value?: string
}

export interface BulkAppResult {
  name: string
  status: 'ok' | 'would_apply' | 'denied' | 'not_found' | 'skipped' | 'error'
  message?: string
  before?: string
  after?: string
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

export type ResourceAction = 'set-memory' | 'set-cpu' | 'set-env'

export const isResourceAction = (a: BulkAction): a is ResourceAction =>
  a === 'set-memory' || a === 'set-cpu' || a === 'set-env'

const MIB = 1024 * 1024

/** Turns "512Mi", "2Gi", "0.5" or "KEY=VALUE" into request fields; null when it does not fit the action. */
export function parseResourceValue(
  action: ResourceAction,
  raw: string,
): Pick<
  BulkRequest,
  'memory_bytes' | 'nano_cpus' | 'env_key' | 'env_value'
> | null {
  const text = raw.trim()
  if (action === 'set-memory') {
    const m = /^(\d+)\s*(Mi|Gi)$/.exec(text)
    if (!m || Number(m[1]) < 1) return null
    return { memory_bytes: Number(m[1]) * (m[2] === 'Gi' ? 1024 * MIB : MIB) }
  }
  if (action === 'set-cpu') {
    const cores = Number(text)
    return text !== '' && Number.isFinite(cores) && cores > 0
      ? { nano_cpus: Math.round(cores * 1e9) }
      : null
  }
  const eq = text.indexOf('=')
  const key = eq > 0 ? text.slice(0, eq) : ''
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(key)
    ? { env_key: key, env_value: text.slice(eq + 1) }
    : null
}
