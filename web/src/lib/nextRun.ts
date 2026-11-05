import { formatAbsolute } from './dateFormat'

/** nextRunDisplay is null when the schedule has no future run (zero or year 1 time). */
export function nextRunDisplay(at: string | null | undefined): string | null {
  if (!at) return null
  const date = new Date(at)
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return null
  if (date.getTime() <= 0) return null
  return formatAbsolute(date)
}
