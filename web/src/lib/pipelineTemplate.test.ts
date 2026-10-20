import { describe, expect, it } from 'vitest'
import { lineOffsets, parseInputLines } from './pipelineTemplate'

describe('parseInputLines', () => {
  it('reads key=value lines and skips blanks and malformed ones', () => {
    expect(
      parseInputLines('env=staging\n\n bad line\nregion = eu=1 \n=x'),
    ).toEqual({
      env: 'staging',
      region: 'eu=1',
    })
  })
})

describe('lineOffsets', () => {
  it('returns the start and end offsets of a 1-based line', () => {
    const text = 'ab\ncde\nf'
    expect(lineOffsets(text, 2)).toEqual([3, 6])
    expect(lineOffsets(text, 99)).toEqual([7, 8])
  })
})
