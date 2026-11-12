// Shared locale checks: the CLI (checkLocale.mjs) and the vitest suites both call checkLanguage.
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
export const LOCALES_DIR = join(here, '..', 'locales')
const BASELINE_DIR = join(here, 'missingBaseline')
const glossary = JSON.parse(readFileSync(join(here, 'glossary.json'), 'utf8'))
const PLURAL = /_(zero|one|two|few|many|other)$/
const DASH = new RegExp('[\\u2013\\u2014]')
// Integers up to this count decide which plural forms a language needs.
const PLURAL_PROBE_MAX = 1000

export function flat(obj, prefix = '', out = {}) {
  for (const [k, v] of Object.entries(obj)) {
    if (v && typeof v === 'object') flat(v, `${prefix}${k}.`, out)
    else out[`${prefix}${k}`] = String(v)
  }
  return out
}

const tokens = (s) =>
  (s.match(/\{\{[^}]+\}\}|<\/?[0-9a-z]+\/?>/gi) ?? []).sort().join('|')

// Plural forms a language needs for whole-number counts. Categories that only apply to
// compact decimals (such as "many" in es and fr) are not reachable by integers and are skipped.
export function requiredPluralCategories(lang) {
  const rules = new Intl.PluralRules(lang)
  const seen = new Set(['other'])
  for (let n = 0; n <= PLURAL_PROBE_MAX; n++) seen.add(rules.select(n))
  return [...seen].sort()
}

export function languageDirs(localesDir = LOCALES_DIR) {
  return readdirSync(localesDir)
    .filter((n) => n !== 'en' && statSync(join(localesDir, n)).isDirectory())
    .sort()
}

// A language with no baseline file has none: everything it ships must be complete and clean.
export function loadBaseline(lang, baselineDir = BASELINE_DIR) {
  const p = join(baselineDir, `${lang}.json`)
  if (!existsSync(p)) return { missing: [], knownIssues: [] }
  const b = JSON.parse(readFileSync(p, 'utf8'))
  return { missing: b.missing ?? [], knownIssues: b.knownIssues ?? [] }
}

function pluralBase(key) {
  return PLURAL.test(key) ? key.replace(PLURAL, '') : null
}

// Returns { files, keys, problems, missing, unlisted, stale } for one language folder.
// missing is every en key (as ns:key) the language lacks in a namespace it ships; unlisted is the
// part not covered by the baseline file, problems excludes baseline knownIssues, and stale is
// baseline entries (missing or knownIssues) that no longer apply.
export function checkLanguage(lang, opts = {}) {
  const localesDir = opts.localesDir ?? LOCALES_DIR
  const only = opts.only ?? []
  const base = opts.baseline ?? loadBaseline(lang, opts.baselineDir)
  const baseline = new Set(base.missing)
  const known = new Set(base.knownIssues)
  const dir = join(localesDir, lang)
  const categories = requiredPluralCategories(lang)
  const raw = []
  const missing = []
  let files = 0
  let keys = 0
  const bad = (m) => raw.push(m)

  for (const f of readdirSync(dir).filter((n) => n.endsWith('.json'))) {
    const ns = f.replace('.json', '')
    if (only.length && !only.includes(ns)) continue
    files++
    const enPath = join(localesDir, 'en', f)
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
      const src =
        en[k] ??
        en[k.replace(PLURAL, '_other')] ??
        en[k.replace(PLURAL, '_one')]
      if (src === undefined) {
        bad(`${ns}:${k}: key not in en`)
        continue
      }
      if (DASH.test(v)) bad(`${ns}:${k}: contains an em or en dash`)
      if (tokens(v) !== tokens(src))
        bad(`${ns}:${k}: placeholders or tags differ from en`)
      for (const t of glossary.doNotTranslate) {
        const re = new RegExp(`(^|[^A-Za-z])${t}([^A-Za-z]|$)`)
        if (re.test(src) && !re.test(v))
          bad(`${ns}:${k}: lost protected term "${t}"`)
      }
    }
    const families = new Set(Object.keys(en).map(pluralBase).filter(Boolean))
    for (const k of Object.keys(en)) {
      const base = pluralBase(k)
      if (base === null) {
        if (!(k in tgt)) missing.push(`${ns}:${k}`)
      } else if (families.has(base)) {
        families.delete(base)
        for (const c of categories)
          if (!(`${base}_${c}` in tgt)) missing.push(`${ns}:${base}_${c}`)
      }
    }
  }
  const now = new Set(missing)
  const rawSet = new Set(raw)
  const checkedNs = (e) => !only.length || only.includes(e.split(':')[0])
  const problems = raw.filter((p) => !known.has(p))
  const unlisted = missing.filter((m) => !baseline.has(m)).sort()
  const stale = [
    ...[...baseline].filter((m) => checkedNs(m) && !now.has(m)),
    ...[...known].filter((m) => checkedNs(m) && !rawSet.has(m)),
  ].sort()
  return { files, keys, problems, missing: [...now].sort(), unlisted, stale }
}
