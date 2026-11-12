export const LOCALES_DIR: string
export interface LanguageCheck {
  files: number
  keys: number
  problems: string[]
  missing: string[]
  unlisted: string[]
  stale: string[]
}
export interface Baseline {
  missing: string[]
  knownIssues: string[]
}
export interface CheckOptions {
  localesDir?: string
  baselineDir?: string
  baseline?: Baseline
  only?: string[]
}
export function flat(
  obj: object,
  prefix?: string,
  out?: Record<string, string>,
): Record<string, string>
export function requiredPluralCategories(lang: string): string[]
export function languageDirs(localesDir?: string): string[]
export function loadBaseline(lang: string, baselineDir?: string): Baseline
export function checkLanguage(lang: string, opts?: CheckOptions): LanguageCheck
