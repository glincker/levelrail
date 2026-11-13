import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import { directionFor, isRtlLanguage, syncDocumentLanguage } from './direction'

describe('rtl helper', () => {
  it.each([
    ['ar', true],
    ['he-IL', true],
    ['fa_IR', true],
    ['en', false],
    ['pt-BR', false],
    ['ja', false],
    [undefined, false],
  ])('isRtlLanguage(%s)', (code, rtl) => {
    expect(isRtlLanguage(code)).toBe(rtl)
    expect(directionFor(code)).toBe(rtl ? 'rtl' : 'ltr')
  })
})

describe('syncDocumentLanguage', () => {
  it('sets lang and dir on init and on every language change', async () => {
    const i = i18next.createInstance()
    await i.init({
      lng: 'en',
      fallbackLng: 'en',
      resources: { en: { translation: {} } },
    })
    syncDocumentLanguage(i)
    expect(document.documentElement.lang).toBe('en')
    expect(document.documentElement.dir).toBe('ltr')
    await i.changeLanguage('ar')
    expect(document.documentElement.lang).toBe('ar')
    expect(document.documentElement.dir).toBe('rtl')
    await i.changeLanguage('pt-BR')
    expect(document.documentElement.lang).toBe('pt-BR')
    expect(document.documentElement.dir).toBe('ltr')
  })
})
