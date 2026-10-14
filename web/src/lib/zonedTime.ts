// Wall-clock helpers for an explicit IANA time zone, so schedule and
// calendar logic is testable without depending on the machine's own zone.

export interface WallTime {
  year: number
  month: number // 1-12
  day: number
  hour: number
  minute: number
}

const DAY_MS = 86_400_000

const formatters = new Map<string, Intl.DateTimeFormat>()

function formatterFor(timeZone: string): Intl.DateTimeFormat {
  let f = formatters.get(timeZone)
  if (!f) {
    f = new Intl.DateTimeFormat('en-US', {
      timeZone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
    formatters.set(timeZone, f)
  }
  return f
}

/** The wall clock in `timeZone` at the instant `ms`. */
export function wallTimeAt(ms: number, timeZone: string): WallTime {
  const parts = formatterFor(timeZone).formatToParts(new Date(ms))
  const get = (type: string) =>
    Number(parts.find((p) => p.type === type)?.value ?? 0)
  return {
    year: get('year'),
    month: get('month'),
    day: get('day'),
    hour: get('hour') % 24,
    minute: get('minute'),
  }
}

function wallAsUtcMs(w: WallTime): number {
  return Date.UTC(w.year, w.month - 1, w.day, w.hour, w.minute)
}

function offsetAt(ms: number, timeZone: string): number {
  const floored = Math.floor(ms / 60_000) * 60_000
  return wallAsUtcMs(wallTimeAt(floored, timeZone)) - floored
}

/**
 * The instant at which `timeZone`'s wall clock reads `w`. An ambiguous time
 * (fall back) resolves to the first occurrence; a time inside a spring
 * forward gap resolves to the same wall time after the gap, matching the
 * server's schedule evaluation.
 */
export function wallTimeToInstant(w: WallTime, timeZone: string): number {
  const guess = wallAsUtcMs(w)
  const before = guess - offsetAt(guess - DAY_MS, timeZone)
  const after = guess - offsetAt(guess + DAY_MS, timeZone)
  const valid = [before, after].filter(
    (c) => wallAsUtcMs(wallTimeAt(c, timeZone)) === guess,
  )
  if (valid.length === 0) return before
  return Math.min(...valid)
}

/** Formats an instant for a datetime-local input in `timeZone`. */
export function toLocalInputValue(ms: number, timeZone: string): string {
  const w = wallTimeAt(ms, timeZone)
  const p = (n: number, len = 2) => String(n).padStart(len, '0')
  return `${p(w.year, 4)}-${p(w.month)}-${p(w.day)}T${p(w.hour)}:${p(w.minute)}`
}

/** Parses a datetime-local value as wall time in `timeZone`; NaN when invalid. */
export function fromLocalInputValue(value: string, timeZone: string): number {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value)
  if (!m) return Number.NaN
  const [, y, mo, d, h, mi] = m
  return wallTimeToInstant(
    {
      year: Number(y),
      month: Number(mo),
      day: Number(d),
      hour: Number(h),
      minute: Number(mi),
    },
    timeZone,
  )
}

/** The browser's own zone, falling back to UTC when it cannot be read. */
export function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}
