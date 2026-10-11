import type { AttentionItem, AttentionTarget } from './attention'

export type AttentionLevel = 'ok' | 'warning' | 'critical'

export interface AttentionSummary {
  level: AttentionLevel
  count: number
  visible: AttentionItem[]
  snoozed: AttentionItem[]
}

export const INSECURE_CONNECTION_ID = 'insecure-connection'

// The one place a shell or dashboard count comes from: info items are
// notifications, not issues, and snoozed items are hidden everywhere.
export function summarizeAttention(
  items: AttentionItem[],
  isSnoozed: (id: string) => boolean,
): AttentionSummary {
  const actionable = items.filter((i) => i.severity !== 'info')
  const visible = actionable.filter((i) => !isSnoozed(i.id))
  const snoozed = actionable.filter((i) => isSnoozed(i.id))
  let level: AttentionLevel = 'ok'
  if (visible.some((i) => i.severity === 'critical')) level = 'critical'
  else if (visible.length > 0) level = 'warning'
  return { level, count: visible.length, visible, snoozed }
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
