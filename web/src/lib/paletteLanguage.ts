import type { i18n as I18n } from 'i18next'
import { supportedLanguages, type LanguageOption } from './languages'

export interface PaletteLanguage extends LanguageOption {
  /** Name in English, so "spanish" finds Español. */
  englishName: string
  current: boolean
}

function englishName(code: string): string {
  try {
    return new Intl.DisplayNames(['en'], { type: 'language' }).of(code) ?? code
  } catch {
    return code
  }
}

/** Active language as a bare code ("es-MX" becomes "es"), matched against the supported list. */
export function currentLanguageCode(i18n: I18n): string {
  const active = i18n.resolvedLanguage ?? i18n.language ?? ''
  const base = active.split('-')[0] ?? active
  const codes = supportedLanguages(i18n).map((l) => l.code)
  return codes.includes(active) ? active : base
}

/** Every supported language with native and English names, read from i18n, never hardcoded. */
export function paletteLanguages(i18n: I18n): PaletteLanguage[] {
  const current = currentLanguageCode(i18n)
  return supportedLanguages(i18n).map((l) => ({
    ...l,
    englishName: englishName(l.code),
    current: l.code === current,
  }))
}

/** The browser's preferred language when it is supported and not the one in use. */
export function browserLanguageOffer(
  i18n: I18n,
  navigatorLanguages: readonly string[],
): PaletteLanguage | null {
  const all = paletteLanguages(i18n)
  const current = currentLanguageCode(i18n)
  for (const tag of navigatorLanguages) {
    const base = tag.split('-')[0]?.toLowerCase() ?? ''
    const match = all.find((l) => l.code === tag || l.code === base)
    if (match) return match.code === current ? null : match
  }
  return null
}
