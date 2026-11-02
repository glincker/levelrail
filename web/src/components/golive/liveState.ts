import type {
  LiveBadgeEntry,
  LiveState,
  StepStatus,
} from '../../queries/goLiveChecklist'
import { FREE_PROVIDER_HOST } from '../../lib/freeDnsHost'

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

// A live domain answers the app, so it outranks a sibling still waiting on DNS.
const APP_STATE_RANK: Record<LiveState, number> = {
  live: 0,
  needs_attention: 1,
  waiting_certificate: 2,
  waiting_dns: 3,
  not_live: 4,
}

/** isAutomaticAddress matches the free wildcard-DNS names Levelrail assigns itself. */
export function isAutomaticAddress(domain: string): boolean {
  return FREE_PROVIDER_HOST.test(domain)
}

/** showsLiveState is false for an automatic address that is not yet proven: it needs no DNS setup, so a waiting badge would mislead. */
export function showsLiveState(
  domain: string,
  state: LiveState,
  automatic = false,
): boolean {
  if (!automatic && !isAutomaticAddress(domain)) return true
  return state === 'live' || state === 'needs_attention'
}

/** pickAppLiveEntry chooses the one domain entry that speaks for the whole app. */
export function pickAppLiveEntry(
  entries: LiveBadgeEntry[] | undefined,
  app: string,
): LiveBadgeEntry | undefined {
  const own =
    entries?.filter(
      (e) => e.app === app && showsLiveState(e.domain, e.state),
    ) ?? []
  return own.reduce<LiveBadgeEntry | undefined>(
    (best, e) =>
      !best || APP_STATE_RANK[e.state] < APP_STATE_RANK[best.state] ? e : best,
    undefined,
  )
}
