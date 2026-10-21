import { describe, expect, it } from 'vitest'
import {
  buildInboxEntries,
  filterInboxEntries,
  inboxUnread,
} from './inboxEntries'
import type { FailedDeploy } from '../queries/failedDeploys'
import type { DeployApprovalResource } from '../types/deployApproval'

const t = (key: string) => key

const failed = {
  id: 'f1',
  service_name: 'web',
  image: 'web:2',
  started_at: '2026-10-20T10:00:00Z',
  finished_at: '2026-10-20T10:05:00Z',
} as FailedDeploy

const approval = {
  id: 'a1',
  service_name: 'api',
  action: 'deploy',
  image: 'api:1',
  requested_by: 'u1',
  requested_by_name: 'Ana',
  created_at: '2026-10-20T11:00:00Z',
} as DeployApprovalResource

describe('sign-in approvals', () => {
  it('become approval entries with a signIn action', () => {
    const [e] = buildInboxEntries({
      t,
      signIns: [
        {
          id: 's1',
          requester_ip: '1.2.3.4',
          user_agent: 'ua',
          created_at: '2026-10-20T12:00:00Z',
          expires_at: '2026-10-20T12:05:00Z',
        },
      ],
    })
    expect(e?.id).toBe('signin:s1')
    expect(e?.kind).toBe('approval')
    expect(e?.action).toEqual({ type: 'signIn', approvalId: 's1' })
  })
})

describe('buildInboxEntries', () => {
  it('merges sources newest first and applies read and archive state', () => {
    const entries = buildInboxEntries({
      t,
      failedDeploys: [failed],
      approvals: [approval],
      feed: [
        {
          id: 'db_upgrade_failed:pg',
          severity: 'warning',
          kind: 'db_upgrade_failed',
          subject: 'pg',
          title: 'Upgrade failed',
          detail: '',
          action: '',
          link: '/databases/pg',
        },
        {
          id: 'token_expiring:x',
          severity: 'warning',
          kind: 'token_expiring',
          subject: 'x',
          title: 'ignored',
          detail: '',
          action: '',
          link: '/',
        },
      ],
      state: { read_ids: ['approval:a1'], archived_ids: ['deploy:f1'] },
    })
    expect(entries.map((e) => e.id)).toEqual([
      'approval:a1',
      'deploy:f1',
      'db_upgrade_failed:pg',
    ])
    expect(entries[0]?.read).toBe(true)
    expect(entries[1]?.archived).toBe(true)
    expect(entries[1]?.action).toEqual({
      type: 'retryDeploy',
      app: 'web',
      image: 'web:2',
    })
    expect(inboxUnread(entries)).toBe(1)
  })

  it('filters by kind, unread and archived', () => {
    const entries = buildInboxEntries({
      t,
      failedDeploys: [failed],
      approvals: [approval],
      state: { read_ids: ['approval:a1'], archived_ids: [] },
    })
    const f = { unreadOnly: false, kind: 'all', archived: false } as const
    expect(filterInboxEntries(entries, { ...f, kind: 'deploy' })).toHaveLength(
      1,
    )
    expect(
      filterInboxEntries(entries, { ...f, unreadOnly: true }),
    ).toHaveLength(1)
    expect(filterInboxEntries(entries, { ...f, archived: true })).toHaveLength(
      0,
    )
  })
})
