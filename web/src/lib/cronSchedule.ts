import { z } from 'zod'

// Shared cron-schedule form logic between BackupScheduleForm (a
// database's backup schedule) and VolumeBackupScheduleForm (an app
// service volume's), extracted so both forms present the exact same
// daily/weekly/custom-cron UI over the exact same internal/cronexpr
// 5-field grammar instead of drifting apart independently. Only exposes
// daily/weekly at a fixed time plus a raw cron escape hatch, not the
// full grammar internal/cronexpr actually parses.

export const WEEKDAY_LABEL: Record<string, string> = {
  '0': 'Sunday',
  '1': 'Monday',
  '2': 'Tuesday',
  '3': 'Wednesday',
  '4': 'Thursday',
  '5': 'Friday',
  '6': 'Saturday',
}

export const scheduleSchema = z
  .object({
    targetId: z.string().trim().min(1, 'Choose a backup target'),
    frequency: z.enum(['daily', 'weekly', 'custom']),
    time: z.string().trim(),
    weekday: z.string().trim(),
    customCron: z.string().trim(),
    retain: z.string().trim(),
    retainDays: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (data.frequency === 'custom') {
      if (data.customCron.split(/\s+/).filter(Boolean).length !== 5) {
        ctx.addIssue({
          code: 'custom',
          message:
            'Cron expression needs 5 fields: minute hour day-of-month month day-of-week',
          path: ['customCron'],
        })
      }
    } else if (!/^([01]\d|2[0-3]):([0-5]\d)$/.test(data.time)) {
      ctx.addIssue({
        code: 'custom',
        message: 'Enter a valid time',
        path: ['time'],
      })
    }
    const retain = Number(data.retain)
    if (!Number.isInteger(retain) || retain < 0) {
      ctx.addIssue({
        code: 'custom',
        message: 'Retention must be a whole number, 0 or more',
        path: ['retain'],
      })
    }
    const retainDays = Number(data.retainDays)
    if (!Number.isInteger(retainDays) || retainDays < 0) {
      ctx.addIssue({
        code: 'custom',
        message: 'Retention must be a whole number, 0 or more',
        path: ['retainDays'],
      })
    }
  })

export type ScheduleFormValues = z.infer<typeof scheduleSchema>

export function toCron(values: ScheduleFormValues): string {
  if (values.frequency === 'custom') {
    return values.customCron.trim()
  }
  const timeParts = values.time.split(':')
  const hour = String(Number(timeParts[0] ?? '0'))
  const minute = String(Number(timeParts[1] ?? '0'))
  if (values.frequency === 'daily') {
    return `${minute} ${hour} * * *`
  }
  return `${minute} ${hour} * * ${values.weekday}`
}

// Inverse of toCron; anything not matching its daily/weekly shapes falls
// back to the custom cron field with the raw string intact.
export function fromCron(
  cron: string | undefined,
): Pick<ScheduleFormValues, 'frequency' | 'time' | 'weekday' | 'customCron'> {
  const raw = cron ?? ''
  const parts = raw.trim().split(/\s+/)
  const isNum = (s: string) => /^\d{1,2}$/.test(s)
  if (parts.length === 5) {
    const minute = parts[0] ?? ''
    const hour = parts[1] ?? ''
    const dom = parts[2] ?? ''
    const month = parts[3] ?? ''
    const dow = parts[4] ?? ''
    if (dom === '*' && month === '*' && isNum(minute) && isNum(hour)) {
      const time = `${hour.padStart(2, '0')}:${minute.padStart(2, '0')}`
      if (dow === '*') {
        return { frequency: 'daily', time, weekday: '0', customCron: raw }
      }
      if (isNum(dow)) {
        return { frequency: 'weekly', time, weekday: dow, customCron: raw }
      }
    }
  }
  return { frequency: 'custom', time: '03:00', weekday: '0', customCron: raw }
}

// scheduleRetentionSummary describes the retain/retainDays pair in the
// same "keeping the last N backups" / "deleting backups older than N
// days" / "keeping every backup" language both forms' summary line uses.
export function scheduleRetentionSummary(
  retain: number | undefined,
  retainDays: number | undefined,
): string {
  const parts: string[] = []
  const r = retain ?? 0
  const rd = retainDays ?? 0
  if (r > 0) {
    parts.push(`keeping the last ${r} backups`)
  }
  if (rd > 0) {
    parts.push(`deleting backups older than ${rd} days`)
  }
  if (r <= 0 && rd <= 0) {
    parts.push('keeping every backup')
  }
  return parts.join(', ')
}
