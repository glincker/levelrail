import { describe, expect, it } from 'vitest'
import type { i18n as I18n } from 'i18next'
import {
  browserLanguageOffer,
  currentLanguageCode,
  paletteLanguages,
} from './paletteLanguage'

const fake = (language: string, supported = ['cimode', 'en', 'es', 'de']) =>
  ({
    language,
    resolvedLanguage: language,
    options: { supportedLngs: supported },
  }) as unknown as I18n

describe('paletteLanguage', () => {
  it('lists supported languages by native and English name', () => {
    const langs = paletteLanguages(fake('en'))
    expect(langs.map((l) => l.code)).toEqual(['en', 'es', 'de'])
    const es = langs.find((l) => l.code === 'es')
    expect(es?.name.toLowerCase()).toContain('espa')
    expect(es?.englishName).toBe('Spanish')
    expect(langs.filter((l) => l.current).map((l) => l.code)).toEqual(['en'])
  })

  it('folds a regional code to its base', () => {
    expect(currentLanguageCode(fake('es-MX'))).toBe('es')
  })

  it('offers the browser language only when supported and different', () => {
    expect(browserLanguageOffer(fake('en'), ['de-DE', 'en'])?.code).toBe('de')
    expect(browserLanguageOffer(fake('de'), ['de-DE'])).toBeNull()
    expect(browserLanguageOffer(fake('en'), ['ja'])).toBeNull()
  })
})
