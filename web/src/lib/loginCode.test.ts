import { describe, expect, it } from 'vitest'
import { normaliseLoginCode } from './loginCode'

describe('normaliseLoginCode', () => {
  it.each([
    ['', ''],
    ['abc', 'ABC'],
    ['abcd', 'ABCD'],
    ['abcde', 'ABCD-E'],
    ['abcd-ef12', 'ABCD-EF12'],
    [' ab cd ef 12 ', 'ABCD-EF12'],
    ['ABCDEF12999', 'ABCD-EF12'],
    ['abcd_ef.12', 'ABCD-EF12'],
  ])('%j becomes %j', (raw, want) => {
    expect(normaliseLoginCode(raw)).toBe(want)
  })
})
