// Client side dry run for parked retention, used for day counts other than
// the saved setting. The server job stays the source of truth.
import type { ParkedDomain } from '../queries/parkedDomains'

export const RETENTION_PRESETS = [7, 30, 90] as const
export const RETENTION_MAX_DAYS = 3650
const DAY_MS = 86_400_000

export interface RetentionCandidate {
  domain: string
  parked_at: string
  age_days: number
}

export interface RetentionEstimate {
  candidates: RetentionCandidate[]
  total: number
  // Domains the first hourly run would delete, oldest first.
  firstRun: RetentionCandidate[]
}

export function clampRetentionDays(raw: number): number {
  if (!Number.isFinite(raw)) return 0
  return Math.min(RETENTION_MAX_DAYS, Math.max(0, Math.floor(raw)))
}

// Rows without a parked_at are never counted: their age is unknown.
export function estimateRetention(
  parked: readonly ParkedDomain[],
  days: number,
  batchLimit: number,
  now: Date,
): RetentionEstimate {
  const empty = { candidates: [], total: 0, firstRun: [] }
  if (days <= 0) return empty
  const candidates: RetentionCandidate[] = []
  for (const p of parked) {
    if (!p.parked_at) continue
    const at = Date.parse(p.parked_at)
    if (Number.isNaN(at)) continue
    const age = Math.floor((now.getTime() - at) / DAY_MS)
    if (age >= days) {
      candidates.push({
        domain: p.domain,
        parked_at: p.parked_at,
        age_days: age,
      })
    }
  }
  candidates.sort((a, b) => b.age_days - a.age_days)
  return {
    candidates,
    total: candidates.length,
    firstRun: candidates.slice(0, Math.max(0, batchLimit)),
  }
}
