import {
  resolveTimeRange,
  type ResolvedTimeRange,
} from '../../../lib/timeRange'

export const PRESET_KEYS = ['15m', '1h', '6h', '24h', '7d'] as const
export type PresetKey = (typeof PRESET_KEYS)[number]
export type MonitoringRangeKey = PresetKey | 'custom'

const HOUR = 60 * 60 * 1000
const DAY = 24 * HOUR

// Same step ladder as lib/timeRange presets, so a custom span stays bounded.
export function stepForSpan(spanMs: number): string | undefined {
  if (spanMs <= HOUR) return undefined
  if (spanMs <= 6 * HOUR) return '60s'
  if (spanMs <= DAY) return '5m'
  if (spanMs <= 7 * DAY) return '30m'
  return '2h'
}

export function maxCustomSpanMs(
  env: Record<string, unknown> = import.meta.env,
): number {
  const raw = Number(env.VITE_MONITORING_MAX_RANGE_DAYS)
  return (Number.isFinite(raw) && raw >= 1 ? raw : 30) * DAY
}

export type CustomRangeResult =
  | { ok: true; range: ResolvedTimeRange }
  | { ok: false; reason: 'invalid' | 'order' | 'future' | 'tooLong' }

export function parseCustomRange(
  fromValue: string,
  toValue: string,
  now: Date,
  maxSpanMs: number,
): CustomRangeResult {
  const from = new Date(fromValue)
  const to = new Date(toValue)
  if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) {
    return { ok: false, reason: 'invalid' }
  }
  if (from.getTime() >= to.getTime()) return { ok: false, reason: 'order' }
  if (to.getTime() > now.getTime() + 60_000) {
    return { ok: false, reason: 'future' }
  }
  const span = to.getTime() - from.getTime()
  if (span > maxSpanMs) return { ok: false, reason: 'tooLong' }
  return { ok: true, range: { from, to, step: stepForSpan(span) } }
}

export function resolvePresetRange(
  key: PresetKey,
  now: Date = new Date(),
): ResolvedTimeRange {
  return resolveTimeRange(key, now)
}

// datetime-local inputs want "YYYY-MM-DDTHH:mm" in local time.
export function toLocalInputValue(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
