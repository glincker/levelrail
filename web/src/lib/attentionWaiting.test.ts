import { describe, expect, it } from 'vitest'
import type { TFunction } from 'i18next'
import type { DeviceAuthRequest } from '../queries/deviceAuth'
import type { NodeResource } from '../types/nodeDetail'
import type { AttentionItem } from './attention'
import { buildWaitingItems, rankAttentionItems } from './attentionWaiting'
import { formatCountdown, liveDeviceRequests } from './deviceLogin'
import { buildNotifications } from '../components/shell/notifications'

const t = ((key: string) => key) as unknown as TFunction<'attention'>
const NOW = Date.parse('2026-10-09T12:00:00Z')

function login(code: string, offsetMs: number): DeviceAuthRequest {
  return {
    user_code: code,
    client_name: 'laptop',
    created_at: new Date(NOW - 1000).toISOString(),
    expires_at: new Date(NOW + offsetMs).toISOString(),
    requester_ip: '203.0.113.9',
    user_agent: 'levelrail-cli',
    ip_mismatch: false,
  }
}

describe('buildWaitingItems', () => {
  it('is empty when nothing waits', () => {
    expect(buildWaitingItems({ now: NOW, t })).toEqual([])
  })

  it('skips expired logins and keeps live ones', () => {
    const items = buildWaitingItems({
      now: NOW,
      t,
      deviceLogins: [login('LIVE-0001', 60_000), login('GONE-0002', -1)],
    })
    expect(items.map((i) => i.id)).toEqual(['device:LIVE-0001'])
  })

  it('flags stalled renewals and never-connected nodes', () => {
    const items = buildWaitingItems({
      now: NOW,
      t,
      certs: [
        {
          domain: 'a.example',
          status: 'healthy',
          renewal: 'stalled',
          not_before: '',
          not_after: '2026-11-01T00:00:00Z',
          source: 'acme',
        },
      ],
      nodes: [
        {
          id: 'n1',
          name: 'edge',
          status_reason: 'enrolled_never_connected',
        } as NodeResource,
      ],
    })
    expect(items.map((i) => i.id)).toEqual([
      'cert-renewal:a.example',
      'node-enroll:n1',
    ])
  })
})

describe('rankAttentionItems', () => {
  const item = (
    id: string,
    severity: AttentionItem['severity'],
  ): AttentionItem => ({
    id,
    severity,
    title: id,
    detail: '',
    target: { kind: 'system' },
  })

  it('puts critical first, then items waiting on a person', () => {
    const ranked = rankAttentionItems([
      item('update', 'warning'),
      item('device:A', 'warning'),
      item('app:web', 'critical'),
      item('approval:1', 'warning'),
    ])
    expect(ranked.map((i) => i.id)).toEqual([
      'app:web',
      'device:A',
      'approval:1',
      'update',
    ])
  })
})

describe('device login helpers', () => {
  it('formats countdowns and clamps at zero', () => {
    expect(formatCountdown(125_000)).toBe('2:05')
    expect(formatCountdown(-5)).toBe('0:00')
  })

  it('orders live requests by soonest expiry', () => {
    const live = liveDeviceRequests(
      [login('B', 120_000), login('A', 30_000), login('C', -10)],
      NOW,
    )
    expect(live.map((r) => r.user_code)).toEqual(['A', 'B'])
  })
})

describe('bell notifications from attention', () => {
  it('lists a pending login under approvals and avoids duplicates', () => {
    const attention = buildWaitingItems({
      now: NOW,
      t,
      deviceLogins: [login('ABCD-1234', 60_000)],
    })
    const list = buildNotifications({
      attention: [
        ...attention,
        {
          id: 'deploy:web',
          severity: 'critical',
          title: 'x',
          detail: '',
          target: { kind: 'system' },
        },
      ],
    })
    expect(list).toHaveLength(1)
    expect(list[0]).toMatchObject({
      group: 'approvals',
      href: '/settings/cli-access?user_code=ABCD-1234',
    })
  })

  it('is empty when nothing needs attention', () => {
    expect(buildNotifications({ attention: [] })).toEqual([])
  })
})
