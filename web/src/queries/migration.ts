// Data and cutover steps after a platform import (internal/api/migration_*.go).
// The source database password only travels in the copy request body and is
// never put in the query cache.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type CopyStatus =
  'pending' | 'copying' | 'verified' | 'failed' | 'unsupported'

export interface CopyTable {
  name: string
  source: number
  target: number
  ok: boolean
}

export interface DatabaseCopy {
  database: string
  engine: string
  status: CopyStatus
  reason?: string
  next_action?: string
  source_host?: string
  source_port?: number
  source_database?: string
  checked: number
  mismatched: number
  tables?: CopyTable[]
}

export interface CopySource {
  host: string
  port?: number
  user?: string
  password?: string
  database?: string
  auth_database?: string
  tls?: boolean
}

export interface CutoverCheck {
  id: string
  status: 'pass' | 'warn' | 'fail'
  detail: string
  fix?: string
}

export interface CutoverChange {
  type: string
  name: string
  value: string
  ttl: number
  replaces?: string
}

export type Verdict = 'go' | 'wait' | 'no-go' | 'switched'

export interface CutoverDomain {
  app: string
  domain: string
  verdict: Verdict
  checks: CutoverCheck[]
  change?: CutoverChange
}

export interface CutoverReport {
  verdict: Verdict
  phase: 'pre-switch' | 'post-switch'
  target_ips: string[]
  domains: CutoverDomain[]
  guidance: string[]
}

export interface VolumeGuide {
  app: string
  kind: 'volume' | 'bind'
  source: string
  target: string
  container_path: string
  command: string
}

export interface VolumeGuideResponse {
  source: string
  guides: VolumeGuide[]
  warning: string
}

export const migrationKeys = {
  copies: ['migration', 'database-copies'] as const,
}

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useDatabaseCopies() {
  return useQuery({
    queryKey: migrationKeys.copies,
    queryFn: () => send<DatabaseCopy[]>('/api/v1/imports/platform/databases'),
    refetchInterval: (q) =>
      q.state.data?.some((d) => d.status === 'copying') ? 3000 : false,
  })
}

export function useStartDatabaseCopy() {
  const queryClient = useQueryClient()
  return useMutation<
    DatabaseCopy,
    ApiError,
    { name: string; source: CopySource }
  >({
    mutationFn: ({ name, source }) =>
      send<DatabaseCopy>(
        `/api/v1/imports/platform/databases/${encodeURIComponent(name)}/copy`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(source),
        },
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: migrationKeys.copies }),
  })
}

export function useCutoverCheck() {
  return useMutation<
    CutoverReport,
    ApiError,
    { verify: boolean; targetIp?: string }
  >({
    mutationFn: ({ verify, targetIp }) => {
      const q = new URLSearchParams()
      if (targetIp) q.set('target_ip', targetIp)
      const qs = q.toString()
      return send<CutoverReport>(
        `/api/v1/migration/cutover${verify ? '/verify' : ''}${qs ? `?${qs}` : ''}`,
      )
    },
  })
}

export function useVolumeGuide() {
  return useMutation<VolumeGuideResponse, ApiError, { source: string }>({
    mutationFn: ({ source }) =>
      send<VolumeGuideResponse>(
        `/api/v1/migration/volumes?source=${encodeURIComponent(source)}`,
      ),
  })
}
