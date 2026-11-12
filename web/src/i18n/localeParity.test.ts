import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { checkLanguage, languageDirs, LOCALES_DIR } from './localeCheck.mjs'

const languages = languageDirs()

// Namespaces behind the first-week routes must ship a Spanish file; extend as more are added.
const FIRST_WEEK = [
  'errors',
  'forms',
  'fixit',
  'envEditor',
  'compose',
  'deploySafety',
  'images',
  'migrate',
]

describe('first-week es namespaces', () => {
  it.skipIf(!languages.includes('es')).each(FIRST_WEEK)(
    'has an es file for %s',
    (ns) => {
      expect(readdirSync(join(LOCALES_DIR, 'es'))).toContain(`${ns}.json`)
    },
  )
})

// Every namespace a language ships must be complete (every en key, and every plural form the
// language needs) unless missingBaseline/<lang>.json lists the gap. No baseline file means no gaps.
describe.each(languages)('en/%s key parity', (lang) => {
  const result = checkLanguage(lang)

  it('has no English keys missing beyond the baseline', () => {
    expect(result.unlisted).toEqual([])
  })

  it('keeps the baseline free of entries that are now fixed', () => {
    expect(result.stale).toEqual([])
  })
})
