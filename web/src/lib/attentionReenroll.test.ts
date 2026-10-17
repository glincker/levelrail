import { describe, expect, it } from 'vitest'
import type { NodeResource } from '../types/nodeDetail'
import type { AttentionItem } from './attention'
import { buildWaitingItems } from './attentionWaiting'

const NOW = Date.parse('2026-10-16T12:00:00Z')
const DAY = 24 * 60 * 60 * 1000
const t = ((k: string, o?: Record<string, string>) =>
  `${k}${o?.date ? `:${o.date}` : ''}`) as never

const node = (leftDays: number, origin = 'reenroll_required') =>
  ({
    id: 'n1',
    name: 'web-1',
    status: 'online',
    cert: {
      state: 'ok',
      generation: 1,
      key_origin: origin,
      not_after: new Date(NOW + leftDays * DAY).toISOString(),
      warning_days: 21,
      critical_days: 7,
    },
  }) as unknown as NodeResource

describe('node re-enroll attention item', () => {
  it.each([
    [90, 'info', 'recommendation'],
    [15, 'warning', 'incident'],
    [3, 'critical', 'incident'],
    [-1, 'critical', 'incident'],
  ] as const)('%s days left gives %s', (days, severity, cls) => {
    const item = buildWaitingItems({
      nodes: [node(days)],
      now: NOW,
      t,
    })[0] as AttentionItem
    expect(item.severity).toBe(severity)
    expect(item.class).toBe(cls)
    expect(item.id).toBe('node_reenroll:n1')
    expect(item.target).toEqual({ kind: 'path', to: '/nodes/n1?reenroll=1' })
    expect(item.detail).toContain(days < 0 ? 'Expired' : 'Detail')
  })

  it('disappears once the node re-enrolled or is gone', () => {
    expect(
      buildWaitingItems({ nodes: [node(3, 'agent')], now: NOW, t }),
    ).toEqual([])
    expect(buildWaitingItems({ nodes: [], now: NOW, t })).toEqual([])
  })
})
