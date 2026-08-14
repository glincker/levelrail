// Shared "last N" time range presets for the metrics dashboard and log
// search panel (TASKS.md 2.4), so both pick from the same vocabulary
// instead of each inventing its own set of range options.

export type TimeRangeKey = '1h' | '6h' | '24h' | '7d'

export interface TimeRangePreset {
  key: TimeRangeKey
  label: string
  durationMs: number
  // Passed as the metrics query's `step` param (a Go duration string,
  // internal/api/metrics.go's parseStep). Omitted for the 1h preset:
  // internal/telemetry's collector samples every 15s (TASKS.md 2.1), so
  // an hour of raw samples is ~240 points per metric, small enough to
  // render unaggregated. Set for longer ranges so both the response body
  // and the number of rendered chart points stay bounded (7 days of raw
  // 15s samples would be ~40,000 points per metric).
  step?: string
}

// Keyed by TimeRangeKey (not a plain array) so resolveTimeRange's lookup
// is statically known to succeed for every TimeRangeKey value, without
// an array .find()/index access that TypeScript's noUncheckedIndexedAccess
// would otherwise widen to `TimeRangePreset | undefined`.
const PRESETS_BY_KEY: Record<TimeRangeKey, TimeRangePreset> = {
  '1h': { key: '1h', label: 'Last hour', durationMs: 60 * 60 * 1000 },
  '6h': {
    key: '6h',
    label: 'Last 6 hours',
    durationMs: 6 * 60 * 60 * 1000,
    step: '60s',
  },
  '24h': {
    key: '24h',
    label: 'Last 24 hours',
    durationMs: 24 * 60 * 60 * 1000,
    step: '5m',
  },
  '7d': {
    key: '7d',
    label: 'Last 7 days',
    durationMs: 7 * 24 * 60 * 60 * 1000,
    step: '30m',
  },
}

const PRESET_ORDER: TimeRangeKey[] = ['1h', '6h', '24h', '7d']

export const TIME_RANGE_PRESETS: TimeRangePreset[] = PRESET_ORDER.map(
  (key) => PRESETS_BY_KEY[key],
)

export const DEFAULT_TIME_RANGE_KEY: TimeRangeKey = '1h'

export interface ResolvedTimeRange {
  from: Date
  to: Date
  step?: string
}

// `now` defaults to the real clock but takes a parameter so callers can
// memoize a stable window (see MetricsDashboard/LogSearchPanel: recomputed
// only on an explicit range-key change or refresh click, not on every
// render, otherwise a fresh `to` on every render would mint a new query
// key and refetch in a loop).
export function resolveTimeRange(
  key: TimeRangeKey,
  now: Date = new Date(),
): ResolvedTimeRange {
  const preset = PRESETS_BY_KEY[key]
  return {
    from: new Date(now.getTime() - preset.durationMs),
    to: now,
    step: preset.step,
  }
}
