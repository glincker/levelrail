// Small pure formatters shared by the observability components.

import type { DeployAttempt } from '../types/deployAttempt'
import type { FailureLogLine, InvestigateSummary } from '../types/investigate'

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

// datetime-local wants local wall time without a zone suffix.
export function toLocalInput(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function formatAttemptDuration(attempt: DeployAttempt): string | null {
  if (!attempt.finished_at) {
    return null
  }
  const ms = Date.parse(attempt.finished_at) - Date.parse(attempt.started_at)
  if (!Number.isFinite(ms) || ms < 0) {
    return null
  }
  const s = Math.round(ms / 1000)
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`
}

export function failureLinesToText(lines: readonly FailureLogLine[]): string {
  return lines.map((l) => `${l.timestamp} ${l.message}`).join('\n')
}

export interface SummaryRow {
  key: 'rate' | 'p50' | 'p95' | 'p99' | 'err4' | 'err5'
  before: string
  during: string
}

export function formatPercent(v: number): string {
  return `${(v * 100).toFixed(1)}%`
}

export function formatMs(v: number): string {
  return `${Math.round(v)} ms`
}

export function summaryRows(
  during: InvestigateSummary,
  before: InvestigateSummary,
): SummaryRow[] {
  return [
    {
      key: 'rate',
      before: `${before.rate_per_sec.toFixed(2)}/s`,
      during: `${during.rate_per_sec.toFixed(2)}/s`,
    },
    {
      key: 'p50',
      before: formatMs(before.p50_ms),
      during: formatMs(during.p50_ms),
    },
    {
      key: 'p95',
      before: formatMs(before.p95_ms),
      during: formatMs(during.p95_ms),
    },
    {
      key: 'p99',
      before: formatMs(before.p99_ms),
      during: formatMs(during.p99_ms),
    },
    {
      key: 'err4',
      before: formatPercent(before.error_rate_4xx),
      during: formatPercent(during.error_rate_4xx),
    },
    {
      key: 'err5',
      before: formatPercent(before.error_rate_5xx),
      during: formatPercent(during.error_rate_5xx),
    },
  ]
}
