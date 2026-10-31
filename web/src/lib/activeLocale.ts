// Dependency-free holder so shared code can read the UI language without importing i18next,
// which main.tsx deliberately loads dynamically to keep it out of the first-load bundle.
let current = 'en'

export function getActiveLocale(): string {
  return current
}

export function setActiveLocale(lang: string | undefined): void {
  if (lang) current = lang
}

type Translator = (key: string, defaultValue: string) => string
let translator: Translator | null = null

// The i18n module registers itself here so shared code can translate without importing it.
export function setTranslator(fn: Translator): void {
  translator = fn
}

export function translateShared(key: string, defaultValue: string): string {
  return translator ? translator(key, defaultValue) : defaultValue
}
