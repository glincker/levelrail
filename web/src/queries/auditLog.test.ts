import { describe, expect, it } from 'vitest'
import { auditLogExportURL, buildAuditLogParams } from './auditLog'

describe('buildAuditLogParams', () => {
  it.each([
    { name: 'empty', opts: {}, want: '' },
    {
      name: 'search is trimmed',
      opts: { search: '  alice ' },
      want: 'q=alice',
    },
    { name: 'blank search omitted', opts: { search: '   ' }, want: '' },
    { name: 'failed only', opts: { failedOnly: true }, want: 'status=failed' },
    {
      name: 'combined with cursor and client',
      opts: {
        before: '2026-01-01T00:00:00Z',
        clientKind: 'cli',
        search: 'a b',
        failedOnly: true,
      },
      want: 'before=2026-01-01T00%3A00%3A00Z&client_kind=cli&q=a+b&status=failed',
    },
  ])('$name', ({ opts, want }) => {
    expect(buildAuditLogParams(opts).toString()).toBe(want)
  })
})

describe('auditLogExportURL', () => {
  it('carries search and failed filters into the csv export', () => {
    const url = auditLogExportURL({ search: 'bob', failedOnly: true })
    expect(url).toContain('format=csv')
    expect(url).toContain('q=bob')
    expect(url).toContain('status=failed')
  })
})
