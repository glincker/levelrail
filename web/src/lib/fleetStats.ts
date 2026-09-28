import type { AppMetricsSummary } from '../types/appsMetrics'
import type { DeployAttempt } from '../types/deployAttempt'

export interface FleetTraffic {
  ratePerSec: number
  errorPct: number
  series: number[]
  reporting: number
}

export function summarizeTraffic(rows: AppMetricsSummary[]): FleetTraffic {
  let rate = 0
  let errWeighted = 0
  let reporting = 0
  let series: number[] = []
  for (const r of rows) {
    if (!r.has_traffic) continue
    reporting += 1
    rate += r.rate_per_sec
    errWeighted += r.rate_per_sec * r.error_rate_5xx
    series = r.spark.map((v, i) => v + (series[i] ?? 0))
  }
  return {
    ratePerSec: rate,
    errorPct: rate > 0 ? (errWeighted / rate) * 100 : 0,
    series,
    reporting,
  }
}

export interface ActivityEntry {
  id: string
  app: string
  at: string
  status: DeployAttempt['status']
  detail: string
}

const HOUR_MS = 3_600_000

export function flattenAttempts(
  byApp: Record<string, DeployAttempt[] | undefined>,
): ActivityEntry[] {
  const out: ActivityEntry[] = []
  for (const [app, attempts] of Object.entries(byApp)) {
    for (const a of attempts ?? []) {
      out.push({
        id: a.id,
        app,
        at: a.finished_at ?? a.started_at,
        status: a.status,
        detail: a.commit_sha ? a.commit_sha.slice(0, 7) : (a.source ?? ''),
      })
    }
  }
  return out.sort((a, b) => b.at.localeCompare(a.at))
}

export function deploysPerHour(
  entries: ActivityEntry[],
  now: number,
  hours = 24,
): { total: number; buckets: number[] } {
  const buckets = new Array<number>(hours).fill(0)
  let total = 0
  for (const e of entries) {
    const age = now - new Date(e.at).getTime()
    if (age < 0 || age >= hours * HOUR_MS) continue
    const slot = hours - 1 - Math.floor(age / HOUR_MS)
    buckets[slot] = (buckets[slot] ?? 0) + 1
    total += 1
  }
  return { total, buckets }
}

export function pickSampleApps<
  T extends { name: string; status: { variant: string } },
>(apps: T[], cap: number): string[] {
  const rank = (a: T) => (a.status.variant === 'destructive' ? 0 : 1)
  return [...apps]
    .sort((a, b) => rank(a) - rank(b))
    .slice(0, cap)
    .map((a) => a.name)
}
