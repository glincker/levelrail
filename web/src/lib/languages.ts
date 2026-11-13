import type { i18n as I18n } from 'i18next'

export interface LanguageOption {
  code: string
  name: string
}

function languageName(code: string): string {
  try {
    const name = new Intl.DisplayNames([code], { type: 'language' }).of(code)
    return name ? name.charAt(0).toLocaleUpperCase(code) + name.slice(1) : code
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

// Maps the detected or regional language (for example es-MX) onto a supported code.
export function resolveSupportedLanguage(i18n: I18n): string {
  const current = i18n.resolvedLanguage ?? i18n.language ?? ''
  const codes = supportedLanguages(i18n).map((l) => l.code)
  const exact = codes.find((c) => c === current)
  if (exact) return exact
  const base = codes.find((c) => current.startsWith(`${c}-`))
  return base ?? codes[0] ?? 'en'
}

const CHOSEN_KEY = 'levelrail-language-chosen'

// A language picked before sign-in waits here until it is saved to the account.
export function rememberChosenLanguage(code: string): void {
  try {
    window.localStorage.setItem(CHOSEN_KEY, code)
  } catch {
    // Without storage the choice still applies for this page view.
  }
}

export function takeChosenLanguage(): string {
  try {
    return window.localStorage.getItem(CHOSEN_KEY) ?? ''
  } catch {
    return ''
  }
}

export function clearChosenLanguage(): void {
  try {
    window.localStorage.removeItem(CHOSEN_KEY)
  } catch {
    // Nothing to clear without storage.
  }
}
