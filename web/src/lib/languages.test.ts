import { describe, expect, it } from 'vitest'
import type { i18n as I18n } from 'i18next'
import { supportedLanguages } from './languages'
import { SUPPORTED_LANGUAGES } from '../i18n/supported'

const fake = {
  options: { supportedLngs: [...SUPPORTED_LANGUAGES, 'cimode'] },
} as unknown as I18n

describe('supportedLanguages', () => {
  const list = supportedLanguages(fake)

  it('lists every supported code except cimode', () => {
    expect(list.map((l) => l.code).sort()).toEqual(
      [...SUPPORTED_LANGUAGES].sort(),
    )
  })

  it('names each language in itself', () => {
    const name = (c: string) => list.find((l) => l.code === c)?.name
    expect(name('es')).toBe('Español')
    expect(name('de')).toBe('Deutsch')
    expect(name('fr')).toBe('Français')
    expect(name('ja')).toBe('日本語')
    expect(name('pt-BR')).toMatch(/portugu/i)
    expect(name('zh-CN')).toMatch(/中文/)
  })

  it('puts English first then sorts by native name', () => {
    expect(list[0]?.code).toBe('en')
    const rest = list.slice(1).map((l) => l.name)
    expect(rest).toEqual([...rest].sort((a, b) => a.localeCompare(b)))
  })
})
