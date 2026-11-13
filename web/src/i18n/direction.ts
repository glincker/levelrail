import type { i18n as I18n } from 'i18next'

// Primary language subtags written right to left.
const RTL_LANGUAGES = new Set([
  'ar',
  'he',
  'fa',
  'ur',
  'ps',
  'sd',
  'ug',
  'yi',
  'dv',
])

export type TextDirection = 'ltr' | 'rtl'

export function isRtlLanguage(code: string | undefined): boolean {
  const primary = (code ?? '').split(/[-_]/)[0]?.toLowerCase() ?? ''
  return RTL_LANGUAGES.has(primary)
}

export function directionFor(code: string | undefined): TextDirection {
  return isRtlLanguage(code) ? 'rtl' : 'ltr'
}

// Keeps <html lang> and dir in step with the active language, including the first one.
export function syncDocumentLanguage(
  i18n: I18n,
  doc: Document = document,
): void {
  const apply = (lng?: string) => {
    const code = lng ?? i18n.language
    if (!code) return
    doc.documentElement.lang = code
    doc.documentElement.dir = i18n.dir(code)
  }
  i18n.on('initialized', () => apply())
  i18n.on('languageChanged', apply)
  apply()
}
