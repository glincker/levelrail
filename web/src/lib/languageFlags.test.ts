import { describe, expect, it } from 'vitest'
import { SUPPORTED_LANGUAGES } from '../i18n/supported'
import { LANGUAGE_FLAGS } from './languageFlags'

describe('LANGUAGE_FLAGS', () => {
  it.each(SUPPORTED_LANGUAGES)('has a complete entry for %s', (code) => {
    const entry = LANGUAGE_FLAGS[code]
    expect(entry?.flag).toBeTruthy()
    expect(entry?.native).toBeTruthy()
    expect(entry?.english).toBeTruthy()
  })

  it('has no entry for an unsupported language', () => {
    expect(Object.keys(LANGUAGE_FLAGS).sort()).toEqual(
      [...SUPPORTED_LANGUAGES].sort(),
    )
  })
})
