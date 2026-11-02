import type { AppVisitors } from '../queries/appVisitors'

export const STATUS_CLASSES = ['2xx', '3xx', '4xx', '5xx'] as const
export type StatusClass = (typeof STATUS_CLASSES)[number]

export const STATUS_TEXT_TONE: Record<StatusClass, string> = {
  '2xx': 'text-tone-success',
  '3xx': 'text-muted-foreground',
  '4xx': 'text-tone-warning',
  '5xx': 'text-tone-danger',
}

export function statusTotal(status: AppVisitors['status']): number {
  return STATUS_CLASSES.reduce((s, c) => s + (status[c] ?? 0), 0)
}

export function statusPercent(
  status: AppVisitors['status'],
  c: StatusClass,
): number {
  const total = statusTotal(status)
  return total > 0 ? Math.round(((status[c] ?? 0) / total) * 100) : 0
}

// True when the stored history starts after the requested window opens.
export function isPartialData(data: AppVisitors): boolean {
  return (
    data.oldest_record !== undefined &&
    Date.parse(data.oldest_record) > Date.parse(data.since)
  )
}
