import type { LiveState, StepStatus } from '../../queries/goLiveChecklist'

export type BadgeVariant =
  'success' | 'warning' | 'destructive' | 'muted' | 'info'

export const STATE_VARIANT: Record<LiveState, BadgeVariant> = {
  live: 'success',
  needs_attention: 'destructive',
  waiting_dns: 'info',
  waiting_certificate: 'info',
  not_live: 'muted',
}

export const STATUS_TONE: Record<StepStatus, string> = {
  ok: 'text-tone-success',
  warn: 'text-tone-warning',
  fail: 'text-tone-danger',
  pending: 'text-tone-info',
  unknown: 'text-muted-foreground',
}
