import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  resetAiChatSeedForTests,
  setAiChatSeed,
  takeAiChatSeed,
} from './aiChatSeed'

describe('aiChatSeed', () => {
  afterEach(() => {
    resetAiChatSeedForTests()
    vi.useRealTimers()
  })

  it('returns the seeded message once, then clears it', () => {
    setAiChatSeed('Help me fix this: web.')
    expect(takeAiChatSeed()).toBe('Help me fix this: web.')
    expect(takeAiChatSeed()).toBeNull()
  })

  it('returns null when nothing was seeded', () => {
    expect(takeAiChatSeed()).toBeNull()
  })

  it('expires a seed that is taken too long after it was set', () => {
    vi.useFakeTimers()
    setAiChatSeed('stale')
    vi.advanceTimersByTime(6 * 60_000)
    expect(takeAiChatSeed()).toBeNull()
  })
})
