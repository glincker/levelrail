import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DatabaseParam {
  name: string
  kind: 'int' | 'float' | 'size' | 'enum' | 'bool'
  default: string
  min?: number
  max?: number
  options?: string[]
  description: string
  current: string
  pending?: string
}

export interface DatabaseParams {
  database: string
  engine: string
  supported: boolean
  params: DatabaseParam[]
  has_pending: boolean
  restart_required: boolean
  staged_by?: string
  staged_at?: string
  previous: Record<string, string>
  max_connections_ceiling: number
}

export interface DatabaseParamsChange {
  set?: Record<string, string>
  unset?: string[]
  rollback?: boolean
}

export const databaseParamsKey = (name: string) =>
  ['databases', 'detail', name, 'params'] as const

async function paramsRequest(
  name: string,
  method: 'GET' | 'PUT' | 'POST' | 'DELETE',
  suffix = '',
  body?: unknown,
): Promise<DatabaseParams> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/params${suffix}`,
    body === undefined
      ? { method }
      : {
          method,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `params request failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseParams
}

export const databaseParamsQuery = (name: string) =>
  queryOptions({
    queryKey: databaseParamsKey(name),
    queryFn: () => paramsRequest(name, 'GET'),
  })

export type ParamsAction =
  | { kind: 'stage'; change: DatabaseParamsChange }
  | { kind: 'discard' }
  | { kind: 'apply' }

export function useDatabaseParamsAction(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (action: ParamsAction) => {
      if (action.kind === 'stage') {
        return paramsRequest(name, 'PUT', '', action.change)
      }
      if (action.kind === 'discard') {
        return paramsRequest(name, 'DELETE', '/pending')
      }
      return paramsRequest(name, 'POST', '/apply', { confirm: true })
    },
    onSuccess: (data) => {
      queryClient.setQueryData(databaseParamsKey(name), data)
    },
  })
}

// buildChange turns edited field values into a staged change against the
// value each field started from.
export function buildChange(
  params: DatabaseParam[],
  edits: Record<string, string>,
): DatabaseParamsChange {
  const set: Record<string, string> = {}
  const unset: string[] = []
  for (const p of params) {
    const edited = edits[p.name]
    if (edited === undefined) continue
    const next = edited.trim()
    const base = p.pending ?? p.current
    if (next === base) continue
    if (next === '') unset.push(p.name)
    else set[p.name] = next
  }
  return { set, unset }
}
