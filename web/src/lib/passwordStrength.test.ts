import { describe, expect, it } from 'vitest'
import { passwordRequirements, scorePassword } from './passwordStrength'

describe('scorePassword', () => {
  it('scores empty and short input low', () => {
    expect(scorePassword('')).toBe(0)
    expect(scorePassword('abc')).toBe(1)
  })

  it('penalises common and repetitive passwords', () => {
    expect(scorePassword('password1234')).toBe(1)
    expect(scorePassword('aaaaaaaaaa')).toBe(1)
  })

  it('rewards length and variety', () => {
    expect(scorePassword('Xq7!mZp2#Lw9vR4k')).toBe(4)
    expect(scorePassword('Xq7!mZp2#Lw9vR4k')).toBeGreaterThan(
      scorePassword('xqmzplwv'),
    )
  })
})

describe('passwordRequirements', () => {
  it('flags each rule', () => {
    expect(passwordRequirements('Abcdefg1!')).toEqual({
      length: true,
      mixedCase: true,
      digit: true,
      symbol: true,
    })
    expect(passwordRequirements('abc').length).toBe(false)
  })
})
