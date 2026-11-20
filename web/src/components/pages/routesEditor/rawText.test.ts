import { describe, expect, it } from 'vitest'
import { diffLines, redirectsToText, textToRedirects } from './rawText'

describe('rawText', () => {
  it('round-trips rules including force and app targets', () => {
    const rows = textToRedirects('/a app://api/x 200!\n/b /c', [])
    expect(rows.map((r) => [r.from, r.to, r.status, r.force])).toEqual([
      ['/a', 'app://api/x', '200', true],
      ['/b', '/c', '301', false],
    ])
    expect(redirectsToText(rows)).toBe('/a app://api/x 200!\n/b /c 301')
  })

  it('diffs added and removed lines', () => {
    const ops = diffLines(['a', 'b'], ['b', 'c'])
    expect(ops.map((o) => o.kind)).toEqual(['del', 'same', 'add'])
  })
})
