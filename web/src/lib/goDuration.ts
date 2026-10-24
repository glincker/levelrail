const UNIT_MS: Record<string, number> = {
  h: 3_600_000,
  m: 60_000,
  s: 1000,
}

const GO_DURATION_PART = /(\d+(?:\.\d+)?)(h|m|s)/g

/** Parses a Go duration such as "6h0m0s" or "15m" to milliseconds; null when it is not one. */
export function parseGoDuration(raw: string): number | null {
  const text = raw.trim()
  if (text === '' || !/^(?:\d+(?:\.\d+)?[hms])+$/.test(text)) return null
  let ms = 0
  for (const m of text.matchAll(GO_DURATION_PART)) {
    ms += Number(m[1]) * (UNIT_MS[m[2] ?? ''] ?? 0)
  }
  return ms
}

interface DurationFormatter {
  format: (parts: Record<string, number>) => string
}

type DurationFormatCtor = new (
  locale: string,
  options: { style: 'long' },
) => DurationFormatter

const HOURS_PER_DAY = 24

function splitMs(ms: number): {
  days: number
  hours: number
  minutes: number
  seconds: number
} {
  const totalSeconds = Math.round(ms / 1000)
  const seconds = totalSeconds % 60
  const totalMinutes = (totalSeconds - seconds) / 60
  const minutes = totalMinutes % 60
  const totalHours = (totalMinutes - minutes) / 60
  return {
    days: Math.floor(totalHours / HOURS_PER_DAY),
    hours: totalHours % HOURS_PER_DAY,
    minutes,
    seconds,
  }
}

/** Locale-aware long duration ("6 hours"); uses Intl.DurationFormat when present, else unit number formats. */
export function formatDuration(ms: number, locale: string): string {
  const parts = splitMs(ms)
  const Ctor = (Intl as unknown as { DurationFormat?: DurationFormatCtor })
    .DurationFormat
  if (Ctor) return new Ctor(locale, { style: 'long' }).format(parts)
  const units: [keyof typeof parts, string][] = [
    ['days', 'day'],
    ['hours', 'hour'],
    ['minutes', 'minute'],
    ['seconds', 'second'],
  ]
  return units
    .filter(([key]) => parts[key] > 0)
    .map(([key, unit]) =>
      new Intl.NumberFormat(locale, {
        style: 'unit',
        unit,
        unitDisplay: 'long',
      }).format(parts[key]),
    )
    .join(' ')
}

/** Formats a Go duration string for display, falling back to the raw text when it cannot be parsed. */
export function formatGoDuration(raw: string, locale: string): string {
  const ms = parseGoDuration(raw)
  return ms === null || ms <= 0 ? raw : formatDuration(ms, locale)
}
