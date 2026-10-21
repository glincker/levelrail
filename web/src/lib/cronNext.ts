// Next-run preview for the job dialog. Mirrors kit/cronexpr (5 fields plus
// the @hourly style shortcuts) and the control plane's wall-clock reading of
// a time zone, so the preview matches what the scheduler will do.

const MACROS: Record<string, string> = {
  '@hourly': '0 * * * *',
  '@daily': '0 0 * * *',
  '@midnight': '0 0 * * *',
  '@weekly': '0 0 * * 0',
  '@monthly': '0 0 1 * *',
  '@yearly': '0 0 1 1 *',
  '@annually': '0 0 1 1 *',
}

export const CRON_MACROS = Object.keys(MACROS)

const MAX_SCAN_DAYS = 800

interface FieldSet {
  values: Set<number>
  star: boolean
}

export interface CronSpec {
  minute: FieldSet
  hour: FieldSet
  dom: FieldSet
  month: FieldSet
  dow: FieldSet
}

function parseTerm(term: string, min: number, max: number, out: Set<number>) {
  const [range = '', stepRaw] = term.split('/')
  const step = stepRaw === undefined ? 1 : Number(stepRaw)
  if (!Number.isInteger(step) || step < 1) throw new Error('bad step')
  let lo: number
  let hi: number
  if (range === '*') {
    lo = min
    hi = max
  } else if (range.includes('-')) {
    const [a = '', b = ''] = range.split('-')
    lo = Number(a)
    hi = Number(b)
  } else {
    lo = Number(range)
    hi = stepRaw === undefined ? lo : max
  }
  if (!Number.isInteger(lo) || !Number.isInteger(hi) || lo > hi) {
    throw new Error('bad range')
  }
  if (lo < min || hi > max) throw new Error('out of range')
  for (let v = lo; v <= hi; v += step) out.add(v)
}

function parseField(raw: string, min: number, max: number): FieldSet {
  const values = new Set<number>()
  for (const term of raw.split(',')) parseTerm(term, min, max, values)
  return { values, star: raw === '*' }
}

// parseCron returns null for anything the control plane would reject.
export function parseCron(expr: string): CronSpec | null {
  const trimmed = expr.trim()
  const full = MACROS[trimmed.toLowerCase()] ?? trimmed
  const f = full.split(/\s+/).filter(Boolean)
  if (f.length !== 5) return null
  try {
    const dow = parseField(f[4] ?? '', 0, 7)
    if (dow.values.has(7)) {
      dow.values.delete(7)
      dow.values.add(0)
    }
    return {
      minute: parseField(f[0] ?? '', 0, 59),
      hour: parseField(f[1] ?? '', 0, 23),
      dom: parseField(f[2] ?? '', 1, 31),
      month: parseField(f[3] ?? '', 1, 12),
      dow,
    }
  } catch {
    return null
  }
}

interface WallParts {
  year: number
  month: number
  day: number
  hour: number
  minute: number
}

const formatters = new Map<string, Intl.DateTimeFormat>()

function zoned(ms: number, tz: string): WallParts {
  let fmt = formatters.get(tz)
  if (!fmt) {
    fmt = new Intl.DateTimeFormat('en-US', {
      timeZone: tz,
      hourCycle: 'h23',
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      hour: 'numeric',
      minute: 'numeric',
    })
    formatters.set(tz, fmt)
  }
  const p: Record<string, number> = {}
  for (const part of fmt.formatToParts(new Date(ms))) {
    if (part.type !== 'literal') p[part.type] = Number(part.value)
  }
  return {
    year: p.year ?? 0,
    month: p.month ?? 0,
    day: p.day ?? 0,
    hour: p.hour ?? 0,
    minute: p.minute ?? 0,
  }
}

function wallAsUtc(w: WallParts): number {
  return Date.UTC(w.year, w.month - 1, w.day, w.hour, w.minute)
}

// instantFor turns a wall time in tz into an instant. A wall time skipped by
// a DST jump runs once, an hour later, like the control plane.
function instantFor(w: WallParts, tz: string): number {
  const target = wallAsUtc(w)
  let guess = target
  for (let i = 0; i < 2; i++) {
    guess = target - (wallAsUtc(zoned(guess, tz)) - guess)
  }
  return wallAsUtc(zoned(guess, tz)) === target ? guess : guess + 3_600_000
}

function dayMatches(spec: CronSpec, y: number, m: number, d: number) {
  const dowOk = spec.dow.values.has(new Date(Date.UTC(y, m - 1, d)).getUTCDay())
  const domOk = spec.dom.values.has(d)
  if (!spec.dom.star && !spec.dow.star) return domOk || dowOk
  return domOk && dowOk
}

// nextRuns lists up to count upcoming instants (ms) strictly after fromMs.
export function nextRuns(
  spec: CronSpec,
  tz: string,
  fromMs: number,
  count: number,
): number[] {
  const out: number[] = []
  const start = zoned(fromMs, tz)
  const hours = [...spec.hour.values].sort((a, b) => a - b)
  const minutes = [...spec.minute.values].sort((a, b) => a - b)
  for (let i = 0; i < MAX_SCAN_DAYS && out.length < count; i++) {
    const day = new Date(Date.UTC(start.year, start.month - 1, start.day + i))
    const y = day.getUTCFullYear()
    const m = day.getUTCMonth() + 1
    const d = day.getUTCDate()
    if (!spec.month.values.has(m) || !dayMatches(spec, y, m, d)) continue
    for (const hour of hours) {
      for (const minute of minutes) {
        const at = instantFor({ year: y, month: m, day: d, hour, minute }, tz)
        if (at > fromMs && !out.includes(at)) out.push(at)
      }
    }
    out.sort((a, b) => a - b)
  }
  return out.slice(0, count)
}

export function isValidTimeZone(tz: string): boolean {
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: tz })
    return true
  } catch {
    return false
  }
}
