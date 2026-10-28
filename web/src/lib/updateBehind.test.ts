import { describe, expect, it } from 'vitest'
import { displayVersion, parseVersion, releasesBehind } from './updateBehind'

describe('releasesBehind', () => {
  it('counts the patch gap inside one minor line', () => {
    expect(releasesBehind('0.8.3', '0.8.6')).toBe(3)
    expect(releasesBehind('v0.8.5', 'v0.8.6')).toBe(1)
  })
  it('returns null when it cannot be counted', () => {
    expect(releasesBehind('0.8.3', '0.9.1')).toBeNull()
    expect(releasesBehind('dev', '0.9.1')).toBeNull()
    expect(releasesBehind('0.8.6', '0.8.6')).toBeNull()
  })
  it('parses and formats versions', () => {
    expect(parseVersion('v1.2.3-rc1')).toEqual({ major: 1, minor: 2, patch: 3 })
    expect(displayVersion('0.8.3+abc')).toBe('v0.8.3')
  })
})
