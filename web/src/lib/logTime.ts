import { getActiveLocale } from './activeLocale'

const UNITS: ReadonlyArray<readonly [Intl.RelativeTimeFormatUnit, number]> = [
  ['day', 86_400],
  ['hour', 3_600],
  ['minute', 60],
  ['second', 1],
]

/** Parses an RFC3339 string or epoch ms into epoch ms, or undefined when unusable. */
export function parseLogTime(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value !== 'string' || value === '') return undefined
  const ms = Date.parse(value)
  return Number.isFinite(ms) ? ms : undefined
}

/** Short, locale-aware relative time with second precision ("12s ago"). */
export function formatLogRelative(at: number, now: number): string {
  const diff = Math.round((at - now) / 1000)
  const abs = Math.abs(diff)
  const rtf = new Intl.RelativeTimeFormat(getActiveLocale(), {
    numeric: 'always',
    style: 'narrow',
  })
  for (const [unit, size] of UNITS) {
    if (abs >= size || unit === 'second') {
      return rtf.format(Math.trunc(diff / size), unit)
    }
  }
  return ''
}

/** Full local date and time with seconds, for the hover title. */
export function formatLogAbsolute(at: number): string {
  return new Intl.DateTimeFormat(getActiveLocale(), {
    dateStyle: 'medium',
    timeStyle: 'long',
  }).format(new Date(at))
}
