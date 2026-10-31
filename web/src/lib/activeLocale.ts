// Dependency-free holder so shared code can read the UI language without importing i18next,
// which main.tsx deliberately loads dynamically to keep it out of the first-load bundle.
let current = 'en'

export function getActiveLocale(): string {
  return current
}

export function setActiveLocale(lang: string | undefined): void {
  if (lang) current = lang
}
