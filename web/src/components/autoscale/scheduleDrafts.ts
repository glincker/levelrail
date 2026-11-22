import type {
  AutoscaleDay,
  AutoscaleScheduleWindow,
} from '../../queries/autoscale'

export const DAYS: AutoscaleDay[] = [
  'mon',
  'tue',
  'wed',
  'thu',
  'fri',
  'sat',
  'sun',
]

export interface ScheduleDraft {
  days: AutoscaleDay[]
  start: string
  end: string
  timezone: string
  min: string
}

const CLOCK = /^([01]\d|2[0-3]):[0-5]\d$/

export function newScheduleDraft(): ScheduleDraft {
  return {
    days: ['mon', 'tue', 'wed', 'thu', 'fri'],
    start: '08:00',
    end: '20:00',
    timezone: '',
    min: '2',
  }
}

export function toScheduleDrafts(
  windows: AutoscaleScheduleWindow[] | undefined,
): ScheduleDraft[] {
  return (windows ?? []).map((w) => ({
    days: w.days ?? [],
    start: w.start,
    end: w.end,
    timezone: w.timezone ?? '',
    min: String(w.min_replicas),
  }))
}

function knownZone(name: string): boolean {
  if (name === '') return true
  try {
    new Intl.DateTimeFormat('en', { timeZone: name })
    return true
  } catch {
    return false
  }
}

/** Returns the API windows, or null when any draft is not usable. */
export function buildSchedules(
  drafts: ScheduleDraft[],
  maxReplicas: number,
): AutoscaleScheduleWindow[] | null {
  const out: AutoscaleScheduleWindow[] = []
  for (const d of drafts) {
    const min = Number(d.min)
    if (
      !CLOCK.test(d.start) ||
      !CLOCK.test(d.end) ||
      d.start === d.end ||
      !Number.isInteger(min) ||
      min < 1 ||
      min > maxReplicas ||
      !knownZone(d.timezone.trim())
    ) {
      return null
    }
    const w: AutoscaleScheduleWindow = {
      start: d.start,
      end: d.end,
      min_replicas: min,
    }
    // Seven days is the same as none, which the API reads as every day.
    if (d.days.length > 0 && d.days.length < 7) w.days = d.days
    if (d.timezone.trim() !== '') w.timezone = d.timezone.trim()
    out.push(w)
  }
  return out
}
