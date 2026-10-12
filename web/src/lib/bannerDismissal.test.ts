import { describe, expect, it } from 'vitest'
import { parseDismissed, withDismissed } from './bannerDismissal'

describe('parseDismissed', () => {
  it.each([
    [null, []],
    ['', []],
    ['not json', []],
    ['{"a":1}', []],
    ['["a",2,"b"]', ['a', 'b']],
  ] as const)('parses %s', (raw, want) => {
    expect(parseDismissed(raw)).toEqual(want)
  })
})

describe('withDismissed', () => {
  it.each([
    [[], 'a', ['a']],
    [['a'], 'a', ['a']],
    [['a'], 'b', ['a', 'b']],
  ] as const)('adds %s', (cur, id, want) => {
    expect(withDismissed(cur, id)).toEqual(want)
  })
})
