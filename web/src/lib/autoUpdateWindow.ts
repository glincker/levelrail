// Maps the friendly night-window controls onto the cron string the
// automatic update policy stores (internal/upgradesched).

export type WindowDays = 'daily' | 'weekdays' | 'weekends' | 'custom'

export interface WindowParts {
  hour: number
  minute: number
  days: WindowDays
}

const DEFAULT_HOUR = 2
const DEFAULT_MINUTE = 0
export const DEFAULT_MINUTES = 180
export const DURATION_CHOICES = [60, 120, 180, 240, 360]

const SIMPLE = /^(\d{1,2}) (\d{1,2}) \* \* (\*|1-5|0,6|6,0)$/
const LEADING = /^(\d{1,2}) (\d{1,2}) /

const DAYS_FIELD: Record<Exclude<WindowDays, 'custom'>, string> = {
  daily: '*',
  weekdays: '1-5',
  weekends: '0,6',
}

function inRange(h: number, m: number): boolean {
  return h >= 0 && h <= 23 && m >= 0 && m <= 59
}

export function parseWindowCron(cron: string): WindowParts {
  const text = cron.trim()
  const simple = SIMPLE.exec(text)
  if (simple) {
    const minute = Number(simple[1])
    const hour = Number(simple[2])
    if (inRange(hour, minute)) {
      const field = simple[3]
      const days =
        field === '*' ? 'daily' : field === '1-5' ? 'weekdays' : 'weekends'
      return { hour, minute, days }
    }
  }
  const lead = LEADING.exec(text)
  if (lead && inRange(Number(lead[2]), Number(lead[1]))) {
    return { hour: Number(lead[2]), minute: Number(lead[1]), days: 'custom' }
  }
  return { hour: DEFAULT_HOUR, minute: DEFAULT_MINUTE, days: 'custom' }
}

export function buildWindowCron(parts: WindowParts, keep: string): string {
  if (parts.days === 'custom') return keep
  return `${parts.minute} ${parts.hour} * * ${DAYS_FIELD[parts.days]}`
}

export function formatClock(hour: number, minute: number): string {
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
}

/** Clock time `minutes` after the start, wrapping past midnight. */
export function windowEnd(
  hour: number,
  minute: number,
  minutes: number,
): string {
  const total = (hour * 60 + minute + minutes) % 1440
  return formatClock(Math.floor(total / 60), total % 60)
}

export function parseClock(
  value: string,
): { hour: number; minute: number } | null {
  const m = /^(\d{2}):(\d{2})$/.exec(value)
  if (!m) return null
  const hour = Number(m[1])
  const minute = Number(m[2])
  return inRange(hour, minute) ? { hour, minute } : null
}

export function isValidTimezone(zone: string): boolean {
  if (zone.trim() === '') return false
  try {
    new Intl.DateTimeFormat(undefined, { timeZone: zone })
    return true
  } catch {
    return false
  }
}
