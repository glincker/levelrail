import { describe, expect, it } from 'vitest'
import { approvalIdOf, isNonHumanEntry } from './agentActivity'
import type { AuditLogEntry } from '../queries/auditLog'

const base: AuditLogEntry = {
  id: '1',
  actor_type: 'session',
  actor_id: 'u1',
  actor_name: 'ana',
  ability: 'deploy',
  method: 'POST',
  path: '/api/v1/deploy-approvals/da_1/approve',
  status_code: 200,
  remote_addr: '',
  created_at: '2026-11-28T00:00:00Z',
  client_kind: 'dashboard',
}

describe('agent activity helpers', () => {
  it('keeps people out and tokens, MCP and AI in', () => {
    expect(isNonHumanEntry(base)).toBe(false)
    expect(isNonHumanEntry({ ...base, actor_type: 'token' })).toBe(true)
    expect(isNonHumanEntry({ ...base, client_kind: 'mcp' })).toBe(true)
    expect(isNonHumanEntry({ ...base, on_behalf_of: 'user:u1' })).toBe(true)
  })
  it('extracts the approval id from a vote path', () => {
    expect(approvalIdOf(base)).toBe('da_1')
    expect(approvalIdOf({ path: '/api/v1/apps' })).toBeUndefined()
  })
})
