import { describe, expect, it } from 'vitest'
import { splitCells } from '../pageHelpers'
import { STARTERS } from './index'

describe('starters', () => {
  it('stay small, self-contained and escape the site name', () => {
    for (const s of STARTERS) {
      const files = s.files('a<b>')
      const bytes = files.reduce((n, f) => n + f.content.length, 0)
      expect(bytes, s.id).toBeLessThan(6 * 1024)
      for (const f of files) {
        expect(f.content, s.id).not.toMatch(/https?:\/\/(?!example)/)
        expect(f.content, s.id).not.toContain('a<b>')
      }
    }
  })
})

describe('splitCells', () => {
  it('always fills the bar when there is data', () => {
    expect(splitCells([90, 5, 4, 1], 24).reduce((a, b) => a + b, 0)).toBe(24)
    expect(splitCells([0, 0, 0, 0], 24)).toEqual([0, 0, 0, 0])
  })
})
