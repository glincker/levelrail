import { describe, expect, it } from 'vitest'
import { cacheCreateOptions, isCacheEngine } from './cacheAccess'

describe('cacheAccess', () => {
  it('recognises redis-compatible engines only', () => {
    expect(isCacheEngine('valkey')).toBe(true)
    expect(isCacheEngine('postgres')).toBe(false)
  })

  it('sends nothing for other engines', () => {
    expect(cacheCreateOptions('postgres', 'off', 'none')).toEqual({})
  })

  it('passes valid choices and falls back to safe defaults', () => {
    expect(cacheCreateOptions('redis', 'optional', 'none')).toEqual({
      tls: 'optional',
      auth: 'none',
    })
    expect(cacheCreateOptions('keydb', undefined, 'bogus')).toEqual({
      tls: 'required',
      auth: 'auto',
    })
  })
})
