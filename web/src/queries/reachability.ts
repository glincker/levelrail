import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/exposure/reach.go, served by GET /api/v1/security/reachability.

export type ReachSeverity = 'high' | 'medium' | 'low' | 'info'
export type ReachKind =
  | 'app_direct_port'
  | 'app_no_shield'
  | 'database_public'
  | 'port_exposed'
  | 'docker_api'

export interface ReachFix {
  kind: 'auto' | 'guided'
  method?: string
  path?: string
  body?: Record<string, string>
  link?: string
  restarts?: boolean
}

export interface ReachFinding {
  id: string
  kind: ReachKind
  severity: ReachSeverity
  subject: { type: 'app' | 'database' | 'host'; name?: string }
  node?: string
  port?: number
  params?: Record<string, string>
  fix?: ReachFix
}

export interface ReachEndpoint {
  kind: 'domain' | 'port'
  value: string
  class?: string
}

export interface ReachSubject {
  type: 'app' | 'database'
  name: string
  endpoints: ReachEndpoint[]
  findings: number
}

export interface ReachReport {
  generated_at: string
  counts: { high: number; medium: number; low: number; info: number }
  subjects: ReachSubject[]
  findings: ReachFinding[]
  rules_readable: boolean
  restricted_ports: number
}

export const reachabilityKey = ['security', 'reachability'] as const

async function call<T>(
  method: string,
  path: string,
  body?: Record<string, string>,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : method === 'POST' ? '{}' : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${path} failed: ${res.status}`),
    )
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T)
}

export function reachabilityQueryOptions() {
  return queryOptions({
    queryKey: reachabilityKey,
    queryFn: () => call<ReachReport>('GET', '/api/v1/security/reachability'),
  })
}

// Re-check drops the server's cached audit so the score is measured again.
export function useRecheckReachability() {
  const qc = useQueryClient()
  return useMutation<ReachReport, ApiError, void>({
    mutationFn: () =>
      call<ReachReport>('GET', '/api/v1/security/reachability?refresh=true'),
    onSuccess: (rep) => {
      qc.setQueryData(reachabilityKey, rep)
      void qc.invalidateQueries({ queryKey: ['security', 'posture'] })
    },
  })
}

// The fix is the same endpoint the app, shield or database pages call.
export function useApplyReachFix() {
  const qc = useQueryClient()
  return useMutation<ReachReport, ApiError, ReachFix>({
    mutationFn: async (fix) => {
      if (fix.kind !== 'auto' || !fix.method || !fix.path) {
        throw new ApiError(400, 'no automatic fix')
      }
      await call<unknown>(fix.method, fix.path, fix.body)
      return call<ReachReport>(
        'GET',
        '/api/v1/security/reachability?refresh=true',
      )
    },
    onSuccess: (rep) => {
      qc.setQueryData(reachabilityKey, rep)
      void qc.invalidateQueries({ queryKey: ['security', 'posture'] })
    },
  })
}
