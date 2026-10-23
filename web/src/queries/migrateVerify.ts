// Migration parity report (internal/api/migration_parity.go). The source
// token only travels in the request body of a check and is never cached.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { MigratePlatform } from './migrateWizard'

export type VerifySeverity = 'blocking' | 'warning' | 'info'
export type VerifyVerdict = 'pass' | 'warn' | 'fail'
export type VerifyArea =
  'apps' | 'domains' | 'env' | 'databases' | 'volumes' | 'jobs' | 'probes'

export interface VerifyDifference {
  category: VerifyArea
  severity: VerifySeverity
  app?: string
  subject: string
  detail: string
  fix: string
}

export interface VerifyTotals {
  category: VerifyArea
  source: number
  target: number
  matched: number
  blocking: number
  warning: number
  info: number
}

export interface VerifyProbeResult {
  url: string
  status?: number
  latency_ms?: number
  tls: boolean
  tls_valid: boolean
  tls_days_left?: number
  redirect_target?: string
  error?: string
}

export interface VerifyProbe {
  app: string
  domain: string
  source: VerifyProbeResult
  target: VerifyProbeResult
}

export interface VerifyReport {
  platform: string
  generated_at: string
  verdict: VerifyVerdict
  counts: { blocking: number; warning: number; info: number }
  categories: VerifyTotals[]
  differences: VerifyDifference[] | null
  probes?: VerifyProbe[]
  notes?: string[]
  can_recheck: boolean
}

export interface VerifyRequest {
  platform?: MigratePlatform
  url?: string
  token?: string
  target_ip?: string
  skip_probes?: boolean
  recheck?: boolean
}

const KEY = ['migrate-verify-latest']

export function useLatestVerify() {
  return useQuery({
    queryKey: KEY,
    retry: false,
    queryFn: async (): Promise<VerifyReport | null> => {
      const res = await fetch('/api/v1/migrations/verify/latest')
      if (res.status === 404) return null
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `verify report failed: ${res.status}`),
        )
      }
      return (await res.json()) as VerifyReport
    },
  })
}

export function useRunVerify() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (req: VerifyRequest): Promise<VerifyReport> => {
      const res = await fetch('/api/v1/migrations/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `verify failed: ${res.status}`),
        )
      }
      return (await res.json()) as VerifyReport
    },
    onSuccess: (report) => {
      qc.setQueryData(KEY, report)
    },
  })
}
