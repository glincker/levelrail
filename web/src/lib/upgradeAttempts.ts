import type { SelfUpgradeAttempt } from '@/queries/selfUpgrade'

const DAY_MS = 86_400_000

export function dedupeReason(detail: string, error: string): string[] {
  const d = detail.trim()
  const e = error.trim()
  if (!d) return e ? [e] : []
  if (!e || d === e || d.includes(e)) return [d]
  if (e.includes(d)) return [e]
  return [d, e]
}

export function attemptDurationMs(a: SelfUpgradeAttempt): number {
  const s = Date.parse(a.started_at)
  const f = Date.parse(a.finished_at)
  return Number.isFinite(s) && Number.isFinite(f) ? Math.max(0, f - s) : 0
}

export function summarizeAttempts(list: SelfUpgradeAttempt[], now: number) {
  const lastOk = list
    .filter((a) => a.outcome === 'succeeded')
    .sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at))[0]
  const failed30 = list.filter(
    (a) =>
      (a.outcome === 'failed' || a.outcome === 'rolled_back') &&
      now - Date.parse(a.started_at) <= 30 * DAY_MS,
  ).length
  return { lastOk, failed30 }
}
