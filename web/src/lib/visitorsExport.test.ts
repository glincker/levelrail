import { describe, expect, it } from 'vitest'
import { visitorsExportFilename } from './visitorsExport'

describe('visitorsExportFilename', () => {
  it('uses the Content-Disposition filename', () => {
    expect(
      visitorsExportFilename(
        'attachment; filename="visitors-web-7d.csv"',
        'web',
        '7d',
      ),
    ).toBe('visitors-web-7d.csv')
  })

  it('falls back to the app and window pattern', () => {
    expect(visitorsExportFilename(null, 'web', '24h')).toBe(
      'visitors-web-24h.csv',
    )
    expect(visitorsExportFilename('attachment', 'web', '1h')).toBe(
      'visitors-web-1h.csv',
    )
  })

  it('drops path separators from the header name', () => {
    expect(
      visitorsExportFilename('attachment; filename="../x.csv"', 'a', '1h'),
    ).toBe('..x.csv')
  })
})
