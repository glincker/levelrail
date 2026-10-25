// Validation for the "Schedule for later" option on the deploy dialog. The
// server owns the real minimum lead and horizon and answers with its own
// message; this only catches an empty, unparsable or past pick up front.

export type ScheduleAtError = 'required' | 'invalid' | 'past'

export interface ScheduleAtResult {
  error: ScheduleAtError | null
  iso: string
}

// value is a datetime-local string ("YYYY-MM-DDTHH:mm") in the viewer's zone.
export function validateScheduleAt(
  value: string,
  now: number,
): ScheduleAtResult {
  if (!value.trim()) return { error: 'required', iso: '' }
  const when = new Date(value)
  if (Number.isNaN(when.getTime())) return { error: 'invalid', iso: '' }
  if (when.getTime() <= now) return { error: 'past', iso: '' }
  return { error: null, iso: when.toISOString() }
}

const HOUR_MS = 3_600_000

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

// datetime-local wants "YYYY-MM-DDTHH:mm" in the viewer's local time.
export function defaultScheduleWhen(now: number): string {
  const d = new Date(now + HOUR_MS)
  d.setMinutes(0, 0, 0)
  if (d.getTime() < now + HOUR_MS) d.setTime(d.getTime() + HOUR_MS)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
