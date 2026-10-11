// URL search params for the app metrics and logs routes. Everything that
// defines a view lives here so a copied URL reproduces it exactly.

import {
  DEFAULT_TIME_RANGE_KEY,
  TIME_RANGE_PRESETS,
  resolveTimeRange,
  type ResolvedTimeRange,
  type TimeRangeKey,
} from './timeRange'

export type MetricsRangeParam = TimeRangeKey | 'custom'

export interface MetricsSearch {
  range?: MetricsRangeParam
  from?: string
  to?: string
  compare?: boolean
  at?: string
}

export type LogsTab = 'live' | 'search' | 'archive'

export interface LogsSearch {
  tab?: LogsTab
  q?: string
  level?: string
  container?: string
  stream?: 'stdout' | 'stderr'
  field?: string[]
  from?: string
  to?: string
}

const PRESET_KEYS = new Set<string>(TIME_RANGE_PRESETS.map((p) => p.key))
const LOG_TABS = new Set<string>(['live', 'search', 'archive'])

function isoOrUndefined(value: unknown): string | undefined {
  if (typeof value !== 'string' || value === '') {
    return undefined
  }
  return Number.isNaN(Date.parse(value)) ? undefined : value
}

// Numbers too: the router JSON-decodes values, so `q=500` arrives as 500.
function stringOrUndefined(value: unknown): string | undefined {
  if (typeof value === 'number') {
    return String(value)
  }
  return typeof value === 'string' && value !== '' ? value : undefined
}

export function parseMetricsSearch(
  raw: Record<string, unknown>,
): MetricsSearch {
  const out: MetricsSearch = {}
  const from = isoOrUndefined(raw.from)
  const to = isoOrUndefined(raw.to)
  if (from && to && Date.parse(from) < Date.parse(to)) {
    out.range = 'custom'
    out.from = from
    out.to = to
  } else if (typeof raw.range === 'string' && PRESET_KEYS.has(raw.range)) {
    out.range = raw.range as TimeRangeKey
  }
  if (raw.compare === true || raw.compare === 'true') {
    out.compare = true
  }
  const at = isoOrUndefined(raw.at)
  if (at) {
    out.at = at
  }
  return out
}

export function parseLogsSearch(raw: Record<string, unknown>): LogsSearch {
  const out: LogsSearch = {}
  if (typeof raw.tab === 'string' && LOG_TABS.has(raw.tab)) {
    out.tab = raw.tab as LogsTab
  }
  const q = stringOrUndefined(raw.q)
  if (q) {
    out.q = q
  }
  const level = stringOrUndefined(raw.level)
  if (level) {
    out.level = level
  }
  const container = stringOrUndefined(raw.container)
  if (container) {
    out.container = container
  }
  if (raw.stream === 'stdout' || raw.stream === 'stderr') {
    out.stream = raw.stream
  }
  const fieldRaw = raw.field
  const fields = (Array.isArray(fieldRaw) ? fieldRaw : [fieldRaw]).filter(
    (f): f is string => typeof f === 'string' && f !== '',
  )
  if (fields.length > 0) {
    out.field = fields
  }
  const from = isoOrUndefined(raw.from)
  const to = isoOrUndefined(raw.to)
  if (from && to) {
    out.from = from
    out.to = to
  }
  return out
}

// resolveMetricsRange turns search params into a concrete window. A custom
// range ignores `now`, so a shared link shows the same data.
export function resolveMetricsRange(
  search: MetricsSearch,
  now: Date = new Date(),
): ResolvedTimeRange {
  if (search.range === 'custom' && search.from && search.to) {
    return { from: new Date(search.from), to: new Date(search.to) }
  }
  const key = search.range && search.range !== 'custom' ? search.range : null
  return resolveTimeRange(key ?? DEFAULT_TIME_RANGE_KEY, now)
}

// investigationWindow centres a window on `at`, one twelfth of the visible
// range wide and clamped to 5 minutes through 1 hour.
export function investigationWindow(
  at: Date,
  range: ResolvedTimeRange,
): { from: Date; to: Date } {
  const MIN = 5 * 60 * 1000
  const MAX = 60 * 60 * 1000
  const span = range.to.getTime() - range.from.getTime()
  const width = Math.min(MAX, Math.max(MIN, span / 12))
  return {
    from: new Date(at.getTime() - width / 2),
    to: new Date(at.getTime() + width / 2),
  }
}
