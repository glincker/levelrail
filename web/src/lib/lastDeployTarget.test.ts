import { afterEach, describe, expect, it, vi } from 'vitest'
import { readLastDeployTarget, writeLastDeployTarget } from './lastDeployTarget'

describe('lastDeployTarget', () => {
  afterEach(() => {
    window.localStorage.clear()
    vi.restoreAllMocks()
  })

  it('keeps the value per user and scope', () => {
    writeLastDeployTarget('a@x.io', 'app', 'n1')
    expect(readLastDeployTarget('a@x.io', 'app')).toBe('n1')
    expect(readLastDeployTarget('b@x.io', 'app')).toBeNull()
  })

  it('survives a throwing storage', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(readLastDeployTarget('a@x.io', 'app')).toBeNull()
  })
})
