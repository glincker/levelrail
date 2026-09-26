import { describe, expect, it } from 'vitest'
import type { FailedDeploy } from '../../queries/failedDeploys'
import type { DeployApprovalResource } from '../../types/deployApproval'
import {
  buildNotifications,
  groupNotifications,
  markAllRead,
  unreadCount,
} from './notifications'
import { buildPaletteSuggestions } from './paletteSuggestions'
import { helpDocForPath } from './contextHelp'

const list = buildNotifications({
  failedDeploys: [
    {
      id: 'd1',
      service_name: 'web',
      started_at: '2026-01-01T00:00:00Z',
    } as FailedDeploy,
  ],
  approvals: [
    {
      id: 'a1',
      service_name: 'api',
      requested_by: 'u',
      requested_by_name: 'Ann',
    } as DeployApprovalResource,
  ],
  activity: [
    {
      id: 'x',
      kind: 'node_offline',
      severity: 'critical',
      title: 'n',
      detail: 'd',
      href: '/nodes/1',
    },
  ],
})

describe('notifications', () => {
  it('groups in fixed order and skips empty groups', () => {
    expect(groupNotifications(list).map((g) => g.group)).toEqual([
      'approvals',
      'deploys',
      'system',
    ])
  })

  it('counts unread and marks all read', () => {
    expect(unreadCount(list, new Set())).toBe(3)
    expect(unreadCount(list, new Set(['deploy:d1']))).toBe(2)
    const read = markAllRead(new Set(['old']), list)
    expect(unreadCount(list, new Set(read))).toBe(0)
    expect(read).toContain('old')
  })

  it('a new failure after mark-all-read is unread again', () => {
    const read = new Set(markAllRead(new Set(), list))
    const next = buildNotifications({
      failedDeploys: [
        { id: 'd2', service_name: 'web', started_at: '' } as FailedDeploy,
      ],
    })
    expect(unreadCount(next, read)).toBe(1)
  })
})

describe('palette suggestion ordering', () => {
  const apps = [
    { name: 'web', failing: false },
    { name: 'api', failing: true },
    { name: 'db-admin', failing: false },
    { name: 'cron', failing: true },
  ]
  it('orders current app actions, failing, recent, assistant', () => {
    const out = buildPaletteSuggestions({
      currentApp: 'web',
      apps,
      recentKeys: [
        'app-db-admin',
        'app-web',
        'nav-apps',
        'app-action-x-api',
        'app-api',
      ],
    })
    expect(out.map((s) => s.key)).toEqual([
      'suggest-restart-web',
      'suggest-deploy-web',
      'suggest-failing-api',
      'suggest-failing-cron',
      'suggest-recent-db-admin',
      'suggest-assistant',
    ])
  })

  it('has only the assistant with no context', () => {
    expect(
      buildPaletteSuggestions({ apps: [], recentKeys: [] }).map((s) => s.kind),
    ).toEqual(['assistant'])
  })
})

describe('contextual help', () => {
  it.each([
    ['/apps/web/domains', 'domains-and-ingress'],
    ['/apps/web/logs', 'observability'],
    ['/databases/pg', 'managing-databases'],
    ['/', undefined],
  ])('%s', (path, doc) => {
    expect(helpDocForPath(path)).toBe(doc)
  })
})
