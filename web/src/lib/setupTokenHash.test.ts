import { describe, expect, it } from 'vitest'
import { setupTokenFromHash } from './setupTokenHash'

describe('setupTokenFromHash', () => {
  it.each([
    ['#setup=abc123', 'abc123'],
    ['setup=abc123', 'abc123'],
    ['#setup=', undefined],
    ['#other=1', undefined],
    ['', undefined],
  ])('%j -> %j', (hash, want) => {
    expect(setupTokenFromHash(hash)).toBe(want)
  })
})
