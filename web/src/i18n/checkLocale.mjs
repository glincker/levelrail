// Usage: node web/src/i18n/checkLocale.mjs <lang> [namespace ...]
// Checks a locale folder against en: valid JSON, no unknown keys, matching {{placeholders}} and tags,
// no em or en dashes, and protected glossary terms preserved. Exits 1 on any problem.
import { readdirSync, readFileSync, existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const locales = join(here, '..', 'locales')
const [lang, ...only] = process.argv.slice(2)
if (!lang) {
  console.error('usage: checkLocale.mjs <lang> [namespace ...]')
  process.exit(2)
}
const glossary = JSON.parse(readFileSync(join(here, 'glossary.json'), 'utf8'))
const PLURAL = /_(zero|one|two|few|many|other)$/
const DASH = new RegExp('[\\u2013\\u2014]')

function flat(obj, prefix = '', out = {}) {
  for (const [k, v] of Object.entries(obj)) {
    if (v && typeof v === 'object') flat(v, `${prefix}${k}.`, out)
    else out[`${prefix}${k}`] = String(v)
  }
  return out
}
const tokens = (s) =>
  (s.match(/\{\{[^}]+\}\}|<\/?[0-9a-z]+\/?>/gi) ?? []).sort().join('|')

const dir = join(locales, lang)
if (!existsSync(dir)) {
  console.error(`no locales/${lang}`)
  process.exit(2)
}
let problems = 0
let files = 0
let keys = 0
const bad = (m) => {
  problems++
  console.error(m)
}
for (const f of readdirSync(dir).filter((n) => n.endsWith('.json'))) {
  const ns = f.replace('.json', '')
  if (only.length && !only.includes(ns)) continue
  files++
  const enPath = join(locales, 'en', f)
  if (!existsSync(enPath)) {
    bad(`${ns}: no en counterpart`)
    continue
  }
  let tgt
  try {
    tgt = flat(JSON.parse(readFileSync(join(dir, f), 'utf8')))
  } catch (e) {
    bad(`${ns}: invalid JSON (${e.message})`)
    continue
  }
  const en = flat(JSON.parse(readFileSync(enPath, 'utf8')))
  for (const [k, v] of Object.entries(tgt)) {
    keys++
    const src = en[k] ?? en[k.replace(PLURAL, '_other')] ?? en[k.replace(PLURAL, '_one')]
    if (src === undefined) {
      bad(`${ns}:${k}: key not in en`)
      continue
    }
    if (DASH.test(v)) bad(`${ns}:${k}: contains an em or en dash`)
    if (tokens(v) !== tokens(src)) bad(`${ns}:${k}: placeholders or tags differ from en`)
    for (const t of glossary.doNotTranslate) {
      const re = new RegExp(`(^|[^A-Za-z])${t}([^A-Za-z]|$)`)
      if (re.test(src) && !re.test(v)) bad(`${ns}:${k}: lost protected term "${t}"`)
    }
  }
}
console.log(`${lang}: ${files} files, ${keys} keys, ${problems} problems`)
process.exit(problems ? 1 : 0)
