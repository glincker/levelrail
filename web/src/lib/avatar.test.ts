import { describe, expect, it } from 'vitest'
import { AVATAR_TONES, avatarInitials, avatarTone, gravatarUrl } from './avatar'

describe('avatarInitials', () => {
  it.each([
    ['Ada Lovelace', 'AL'],
    ['ada', 'A'],
    ['grace.hopper@example.com', 'GH'],
    ['  ', '?'],
    ['', '?'],
    ['Jean-Luc Picard', 'JP'],
  ])('%s -> %s', (input, want) => {
    expect(avatarInitials(input)).toBe(want)
  })
})

describe('avatarTone', () => {
  it('is stable for a seed and always one of the palette', () => {
    const a = avatarTone('user_123')
    expect(avatarTone('user_123')).toBe(a)
    expect(AVATAR_TONES).toContain(a)
  })

  it('spreads different seeds over more than one tone', () => {
    const tones = new Set(
      Array.from({ length: 40 }, (_, i) => avatarTone(`user_${String(i)}`)),
    )
    expect(tones.size).toBeGreaterThan(1)
  })
})

describe('gravatarUrl', () => {
  it('hashes the trimmed lower-cased email and never sends it in clear', async () => {
    const a = await gravatarUrl('  Ada@Example.com ')
    const b = await gravatarUrl('ada@example.com')
    expect(a).toBe(b)
    expect(a).not.toContain('example.com')
    expect(a).toMatch(
      /^https:\/\/www\.gravatar\.com\/avatar\/[0-9a-f]{64}\?s=96&d=404$/,
    )
  })
})
