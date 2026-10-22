import { describe, expect, it } from 'vitest'
import { splitByNeedle } from './logHighlight'

describe('splitByNeedle', () => {
  it('returns one plain segment for an empty needle', () => {
    expect(splitByNeedle('abc', '  ')).toEqual([{ text: 'abc', match: false }])
  })

  it('marks every case-insensitive match', () => {
    expect(splitByNeedle('Error: error', 'ERR')).toEqual([
      { text: 'Err', match: true },
      { text: 'or: ', match: false },
      { text: 'err', match: true },
      { text: 'or', match: false },
    ])
  })
})
