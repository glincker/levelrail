import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import en from '../locales/en/common.json'
import { checkLanguage, languageDirs, LOCALES_DIR } from './localeCheck.mjs'
import { SUPPORTED_LANGUAGES } from './supported'

// A language folder that has not landed yet is simply absent here and falls back to en.
const languages = languageDirs()

function only(problems: string[], re: RegExp): string[] {
  return problems.filter((p) => re.test(p))
}

describe('locale folders', () => {
  it('are all registered as supported languages', () => {
    expect(languages.filter((l) => !SUPPORTED_LANGUAGES.includes(l))).toEqual(
      [],
    )
  })

  it('never list en itself', () => {
    expect(languages).not.toContain('en')
    expect(LOCALES_DIR).toContain('locales')
  })
})

describe.each(languages)('%s locale', (lang) => {
  const result = checkLanguage(lang)

  it('is valid JSON and every file has an en counterpart', () => {
    expect(only(result.problems, /invalid JSON|no en counterpart/)).toEqual([])
  })

  it('only has keys that exist in English', () => {
    expect(only(result.problems, /key not in en/)).toEqual([])
  })

  it('keeps every interpolation placeholder and tag', () => {
    expect(only(result.problems, /placeholders or tags/)).toEqual([])
  })

  it('uses no em or en dashes', () => {
    expect(only(result.problems, /em or en dash/)).toEqual([])
  })

  it('keeps protected glossary terms', () => {
    expect(only(result.problems, /protected term/)).toEqual([])
  })

  it('passes every other check', () => {
    expect(result.problems).toEqual([])
  })

  it('falls back to English for keys it does not translate', async () => {
    const i = i18next.createInstance()
    const tgt = (await import(`../locales/${lang}/common.json`)) as {
      default: object
    }
    // cliConsole is emptied so the test keeps proving the fallback even once it is translated.
    await i.init({
      lng: lang,
      fallbackLng: 'en',
      defaultNS: 'common',
      resources: {
        en: { common: en },
        [lang]: { common: { ...tgt.default, cliConsole: {} } },
      },
      interpolation: { escapeValue: false },
    })
    expect(i.t('cliConsole.title')).toBe(en.cliConsole.title)
  })
})
