import { describe, expect, it } from 'vitest'
import type { TFunction } from 'i18next'
import type { DeviceActivityItem } from '../queries/deviceAuth'
import type { AttentionFeedItem } from '../queries/attentionFeed'
import { buildWaitingItems, rankAttentionItems } from './attentionWaiting'
import { buildNotifications } from '../components/shell/notifications'

const t = ((key: string) => key) as unknown as TFunction<'attention'>
const NOW = Date.parse('2026-10-09T12:00:00Z')

function activity(over: Partial<DeviceActivityItem>): DeviceActivityItem {
  return {
    id: 'r1',
    state: 'expired',
    client_name: 'ci',
    requester_ip: '203.0.113.9',
    user_agent: '',
    created_at: new Date(NOW - 7_200_000).toISOString(),
    expires_at: new Date(NOW - 6_600_000).toISOString(),
    ip_mismatch: false,
    dismissible: true,
    dismissed: false,
    item_key: 'device_login:r1:expired',
    audit_path: '/api/v1/auth/device/requests/r1',
    ...over,
  }
}

describe('resolved CLI logins', () => {
  it('keeps expired and denied, drops dismissed, approved and waiting', () => {
    const items = buildWaitingItems({
      now: NOW,
      t,
      resolvedLogins: [
        activity({}),
        activity({ id: 'r2', state: 'denied', item_key: 'k2' }),
        activity({ id: 'r3', dismissed: true, item_key: 'k3' }),
        activity({ id: 'r4', state: 'approved', item_key: 'k4' }),
        activity({ id: 'r5', state: 'waiting', item_key: 'k5' }),
      ],
    })
    expect(items.map((i) => i.id)).toEqual([
      'device-resolved:r1:expired',
      'device-resolved:r2:denied',
    ])
    expect(items.every((i) => i.severity === 'info')).toBe(true)
    expect(items[0]?.device?.dismissKey).toBe('device_login:r1:expired')
  })

  it('ranks info after warnings and critical first', () => {
    const items = rankAttentionItems([
      ...buildWaitingItems({ now: NOW, t, resolvedLogins: [activity({})] }),
      {
        id: 'doctor:x',
        severity: 'warning',
        title: 'w',
        detail: '',
        target: { kind: 'system' },
      },
      {
        id: 'app:web',
        severity: 'critical',
        title: 'c',
        detail: '',
        target: { kind: 'system' },
      },
    ])
    expect(items.map((i) => i.severity)).toEqual([
      'critical',
      'warning',
      'info',
    ])
  })

  it('puts a dismissable bell entry in the approvals group', () => {
    const attention = buildWaitingItems({
      now: NOW,
      t,
      resolvedLogins: [activity({})],
    })
    const [n] = buildNotifications({ attention })
    expect(n?.group).toBe('approvals')
    expect(n?.dismissKey).toBe('device_login:r1:expired')
  })
})

describe('server feed items', () => {
  const feed: AttentionFeedItem[] = [
    {
      id: 'token_expiring:ci:t1',
      severity: 'warning',
      kind: 'token_expiring',
      subject: 'ci',
      title: 'API token ci expires soon',
      detail: '',
      action: 'Create a replacement token',
      link: '/settings/tokens',
      params: { name: 'ci', at: '2026-10-11T00:00:00Z' },
    },
    {
      id: 'backup_overdue:main',
      severity: 'warning',
      kind: 'backup_overdue',
      subject: 'main',
      title: 'Backup of main is overdue',
      detail: 'late',
      action: 'Open the database',
      link: '/databases/main',
      params: { since: '2026-10-01T00:00:00Z' },
    },
    {
      id: 'mystery:x',
      severity: 'info',
      kind: 'mystery',
      subject: 'x',
      title: 'Server title',
      detail: 'server detail',
      action: '',
      link: '/status',
    },
  ]

  it('maps each kind to a deep link and keeps stable ids', () => {
    const items = buildWaitingItems({ now: NOW, t, feed })
    expect(items.map((i) => i.id)).toEqual(feed.map((f) => f.id))
    expect(items[0]?.target).toEqual({ kind: 'path', to: '/settings/tokens' })
    expect(items[1]?.target).toEqual({ kind: 'path', to: '/databases/main' })
    expect(items[2]?.title).toBe('Server title')
  })
})
