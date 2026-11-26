import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import { lazyBackend } from './lazyBackend'
import { SUPPORTED_LANGUAGES } from './supported'
import { checkLanguage } from './localeCheck.mjs'
import { LANGUAGE_FLAGS } from '../lib/languageFlags'
import { supportedLanguages } from '../lib/languages'
import { normalizeDetectedLanguage } from './detect'
import { htmlLang, syncDocumentLanguage } from './direction'

async function makeI18n(lng: string) {
  const i = i18next.createInstance()
  await i.use(lazyBackend).init({
    lng,
    fallbackLng: 'en',
    supportedLngs: SUPPORTED_LANGUAGES,
    nonExplicitSupportedLngs: true,
    ns: ['common'],
    defaultNS: 'common',
  })
  return i
}

describe('en-GB overlay', () => {
  it('passes the locale checker with no missing-key requirement', () => {
    const r = checkLanguage('en-GB')
    expect(r.problems).toEqual([])
    expect(r.missing).toEqual([])
  })

  it('uses British spelling for en-GB and American for en', async () => {
    const uk = await makeI18n('en-GB')
    const us = await makeI18n('en')
    await Promise.all([
      uk.loadNamespaces('deploys'),
      us.loadNamespaces('deploys'),
    ])
    expect(uk.t('deployList.status.canceled', { ns: 'deploys' })).toBe(
      'Cancelled',
    )
    expect(us.t('deployList.status.canceled', { ns: 'deploys' })).toBe(
      'Canceled',
    )
  })

  it('falls back to en for namespaces and keys the overlay lacks', async () => {
    const uk = await makeI18n('en-GB')
    await uk.loadNamespaces('firewall')
    expect(uk.t('cancel', { ns: 'firewall' })).toBe('Cancel')
  })

  it('switching language reloads the namespace', async () => {
    const i = await makeI18n('en')
    await i.changeLanguage('en-GB')
    expect(i.resolvedLanguage).toBe('en-GB')
    await i.changeLanguage('en')
    expect(i.resolvedLanguage).toBe('en')
  })
})

describe('English entries', () => {
  it('lists US then UK English first with their own flags', async () => {
    const i = await makeI18n('en')
    const [first, second] = supportedLanguages(i)
    expect(first?.code).toBe('en')
    expect(second?.code).toBe('en-GB')
    expect(LANGUAGE_FLAGS.en?.native).toBe('English (US)')
    expect(LANGUAGE_FLAGS['en-GB']?.native).toBe('English (UK)')
    expect(LANGUAGE_FLAGS.en?.flag).not.toBe(LANGUAGE_FLAGS['en-GB']?.flag)
  })

  it('detects British and Commonwealth tags as en-GB, US as en', () => {
    expect(normalizeDetectedLanguage('en-GB')).toBe('en-GB')
    expect(normalizeDetectedLanguage('en-AU')).toBe('en-GB')
    expect(normalizeDetectedLanguage('en-US')).toBe('en')
  })

  it('sets the document lang to en-US or en-GB', async () => {
    const i = await makeI18n('en')
    syncDocumentLanguage(i)
    expect(document.documentElement.lang).toBe('en-US')
    await i.changeLanguage('en-GB')
    expect(document.documentElement.lang).toBe('en-GB')
    expect(htmlLang('de')).toBe('de')
  })
})
