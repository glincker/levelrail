import { attentionClassOf } from './attention'
import type { AttentionItem, AttentionTarget } from './attention'

export type AttentionLevel = 'ok' | 'warning' | 'critical'

export interface AttentionSummary {
  level: AttentionLevel
  // Visible incidents only: the one number the header chip and the
  // dashboard status line show.
  count: number
  visible: AttentionItem[]
  snoozed: AttentionItem[]
  // Setup and hardening advice, kept out of count and level.
  recommendations: AttentionItem[]
  snoozedRecommendations: AttentionItem[]
}

export const INSECURE_CONNECTION_ID = 'insecure-connection'

// The one place a shell or dashboard count comes from: info items are
// notifications, not issues, recommendations never count, and snoozed
// items are hidden everywhere.
export function summarizeAttention(
  items: AttentionItem[],
  isSnoozed: (id: string) => boolean,
): AttentionSummary {
  const actionable = items.filter((i) => i.severity !== 'info')
  const incidents = actionable.filter((i) => attentionClassOf(i) === 'incident')
  const advice = actionable.filter(
    (i) => attentionClassOf(i) === 'recommendation',
  )
  const visible = incidents.filter((i) => !isSnoozed(i.id))
  const snoozed = incidents.filter((i) => isSnoozed(i.id))
  let level: AttentionLevel = 'ok'
  if (visible.some((i) => i.severity === 'critical')) level = 'critical'
  else if (visible.length > 0) level = 'warning'
  return {
    level,
    count: visible.length,
    visible,
    snoozed,
    recommendations: advice.filter((i) => !isSnoozed(i.id)),
    snoozedRecommendations: advice.filter((i) => isSnoozed(i.id)),
  }
}

export function insecureConnectionItem(
  title: string,
  detail: string,
): AttentionItem {
  return {
    id: INSECURE_CONNECTION_ID,
    severity: 'warning',
    title,
    detail,
    target: { kind: 'path', to: '/domains' },
    class: 'recommendation',
  }
}

export function attentionHref(target: AttentionTarget): string {
  switch (target.kind) {
    case 'app':
      return `/apps/${encodeURIComponent(target.name)}/overview`
    case 'deploy':
      return `/apps/${encodeURIComponent(target.app)}/deploys`
    case 'node':
      return `/nodes/${encodeURIComponent(target.id)}`
    case 'domain':
      return '/domains'
    case 'system':
      return '/status'
    case 'route':
    case 'path':
      return target.to
  }
}

const SIGN_IN_ID_PREFIXES = ['login_code:', 'login_approval:']

export function isSignInItem(i: AttentionItem): boolean {
  return SIGN_IN_ID_PREFIXES.some((p) => i.id.startsWith(p))
}

// Drops items another surface already shows, keeping count and level in step.
export function withoutItems(
  s: AttentionSummary,
  drop: (i: AttentionItem) => boolean,
): AttentionSummary {
  const visible = s.visible.filter((i) => !drop(i))
  let level: AttentionLevel = 'ok'
  if (visible.some((i) => i.severity === 'critical')) level = 'critical'
  else if (visible.length > 0) level = 'warning'
  return {
    ...s,
    level,
    count: visible.length,
    visible,
    snoozed: s.snoozed.filter((i) => !drop(i)),
  }
}
