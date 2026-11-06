import { describe, expect, it } from 'vitest'
import { auditLogExportURL, buildAuditLogParams } from './auditLog'

describe('audit investigation filters', () => {
  it('carries since, until and actor into the query and the export', () => {
    const opts = {
      since: '2026-01-01T00:00:00Z',
      until: '2026-01-02T00:00:00Z',
      actor: ' alice ',
    }
    expect(buildAuditLogParams(opts).toString()).toBe(
      'since=2026-01-01T00%3A00%3A00Z&until=2026-01-02T00%3A00%3A00Z&actor=alice',
    )
    expect(auditLogExportURL(opts)).toContain('actor=alice')
  })
})
