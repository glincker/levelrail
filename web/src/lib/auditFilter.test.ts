import { describe, expect, it } from 'vitest'
import type { AuditLogEntry } from '../queries/auditLog'
import { filterAuditEntries } from './auditFilter'

const e = (
  id: string,
  actor_name: string,
  path: string,
  status_code: number,
): AuditLogEntry => ({
  id,
  actor_type: 'user',
  actor_id: id,
  actor_name,
  ability: 'apps.write',
  method: 'POST',
  path,
  status_code,
  remote_addr: '10.0.0.1',
  created_at: '2026-09-23T00:00:00Z',
  client_kind: 'web',
})

const entries = [
  e('1', 'alice', '/api/v1/apps/web/restart', 200),
  e('2', 'bob', '/api/v1/apps/api/deploy', 500),
  e('3', 'alice', '/api/v1/domains', 403),
]

describe('filterAuditEntries', () => {
  it.each([
    { name: 'no filter', text: '', failed: false, ids: ['1', '2', '3'] },
    { name: 'text on actor', text: 'ALICE', failed: false, ids: ['1', '3'] },
    { name: 'text on path', text: 'deploy', failed: false, ids: ['2'] },
    { name: 'failed only', text: '', failed: true, ids: ['2', '3'] },
    { name: 'both', text: 'alice', failed: true, ids: ['3'] },
  ])('$name', ({ text, failed, ids }) => {
    expect(filterAuditEntries(entries, text, failed).map((x) => x.id)).toEqual(
      ids,
    )
  })
})
