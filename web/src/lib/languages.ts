import type { i18n as I18n } from 'i18next'

export interface LanguageOption {
  code: string
  name: string
}

function languageName(code: string): string {
  try {
    return new Intl.DisplayNames([code], { type: 'language' }).of(code) ?? code
  } catch {
    return code
  }
}

// Offered languages are whatever the i18n setup supports, named in themselves: English first,
// then alphabetical by native name.
export function supportedLanguages(i18n: I18n): LanguageOption[] {
  const supported = i18n.options.supportedLngs
  const codes: readonly string[] = supported
    ? supported.filter((c) => c !== 'cimode')
    : ['en']
  return codes
    .map((code) => ({ code, name: languageName(code) }))
    .sort((a, b) =>
      a.code === 'en' ? -1 : b.code === 'en' ? 1 : a.name.localeCompare(b.name),
    )
}

export function isSupportedLanguage(i18n: I18n, code: string): boolean {
  return supportedLanguages(i18n).some((l) => l.code === code)
}
