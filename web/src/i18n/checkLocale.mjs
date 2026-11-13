// Usage: node web/src/i18n/checkLocale.mjs <lang> [namespace ...]
// Checks a locale folder against en (see localeCheck.mjs): valid JSON, no unknown keys, matching
// placeholders and tags, no em or en dashes, protected glossary terms, plural forms, and no missing
// keys beyond missingBaseline/<lang>.json. Exits 1 on any problem.
import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { LOCALES_DIR, checkLanguage } from './localeCheck.mjs'

const [lang, ...only] = process.argv.slice(2)
if (!lang) {
  console.error('usage: checkLocale.mjs <lang> [namespace ...]')
  process.exit(2)
}
if (!existsSync(join(LOCALES_DIR, lang))) {
  console.error(`no locales/${lang}`)
  process.exit(2)
}
const r = checkLanguage(lang, { only })
for (const p of r.problems) console.error(p)
for (const m of r.unlisted)
  console.error(
    `${m}: missing (translate it or list it in missingBaseline/${lang}.json)`,
  )
for (const m of r.stale)
  console.error(
    `${m}: listed in missingBaseline/${lang}.json but already translated`,
  )
const total = r.problems.length + r.unlisted.length + r.stale.length
console.log(`${lang}: ${r.files} files, ${r.keys} keys, ${total} problems`)
process.exit(total ? 1 : 0)
