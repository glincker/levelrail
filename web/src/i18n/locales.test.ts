import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import en from '../locales/en/common.json'
import es from '../locales/es/common.json'

function leafKeys(obj: object, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([k, v]) =>
    typeof v === 'object' && v !== null
      ? leafKeys(v as object, `${prefix}${k}.`)
      : [`${prefix}${k}`],
  )
}

function placeholders(s: string): string[] {
  return [...s.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1] ?? '').sort()
}

function lookup(obj: object, key: string): string {
  return key
    .split('.')
    .reduce<unknown>(
      (cur, part) => (cur as Record<string, unknown>)[part],
      obj,
    ) as string
}

describe('es locale', () => {
  const enKeys = new Set(leafKeys(en))
  const esKeys = leafKeys(es)

  it('only has keys that exist in English', () => {
    expect(esKeys.filter((k) => !enKeys.has(k))).toEqual([])
  })

  it('keeps every interpolation placeholder', () => {
    const bad = esKeys.filter(
      (k) =>
        placeholders(lookup(es, k)).join() !==
        placeholders(lookup(en, k)).join(),
    )
    expect(bad).toEqual([])
  })

  it('uses no em or en dashes', () => {
    expect(JSON.stringify(es)).not.toMatch(new RegExp('[\\u2013\\u2014]'))
  })

  it('falls back to English for keys it does not translate', async () => {
    const i = i18next.createInstance()
    await i.init({
      lng: 'es',
      fallbackLng: 'en',
      defaultNS: 'common',
      resources: { en: { common: en }, es: { common: es } },
      interpolation: { escapeValue: false },
    })
    expect(i.t('actions.copy')).toBe('Copiar')
    expect(i.t('cliConsole.title')).toBe(en.cliConsole.title)
  })
})
