#!/usr/bin/env node
// Per-chunk size report and budget gate. Reads dist/assets after `vite build`.
//   node scripts/bundle-report.js            report the largest chunks, fail over budget
//   node scripts/bundle-report.js --update   rewrite bundle-budgets.json from this build
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import path from 'node:path'

const root = path.resolve(import.meta.dirname, '..')
const assets = path.join(root, 'dist', 'assets')
const budgetFile = path.join(root, 'bundle-budgets.json')

const MIN_BUDGETED_BYTES = 30_000
const HEADROOM = 0.15
const TOP_N = 20

// Rollup names a chunk "<name>-<8 char hash>.js"; mermaid's own diagram
// chunks carry an uppercase id first and are lazy, so they are not budgeted.
function chunkName(file) {
  const base = file.replace(/\.js$/, '')
  const m = /^(.*)-[A-Za-z0-9_-]{8}$/.exec(base)
  return m ? m[1] : base
}

function isThirdPartyDiagram(name) {
  return /-[A-Z0-9]{8}$/.test(name)
}

function toKb(bytes) {
  return `${(bytes / 1000).toFixed(1)} kB`
}

let files
try {
  files = readdirSync(assets).filter((f) => f.endsWith('.js'))
} catch {
  console.error('[bundle-report] no build output, run `npm run build` first')
  process.exit(1)
}

const sizes = new Map()
for (const f of files) {
  const name = chunkName(f)
  if (isThirdPartyDiagram(name)) continue
  const size = statSync(path.join(assets, f)).size
  sizes.set(name, Math.max(sizes.get(name) ?? 0, size))
}

if (process.argv.includes('--update')) {
  const chunks = {}
  for (const [name, size] of [...sizes].sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    if (size >= MIN_BUDGETED_BYTES) {
      chunks[name] = Math.ceil((size * (1 + HEADROOM)) / 1000) * 1000
    }
  }
  writeFileSync(budgetFile, `${JSON.stringify({ chunks }, null, 2)}\n`)
  console.log(`[bundle-report] wrote ${Object.keys(chunks).length} budgets`)
  process.exit(0)
}

let budgets = {}
try {
  budgets = JSON.parse(readFileSync(budgetFile, 'utf8')).chunks ?? {}
} catch {
  console.error(`[bundle-report] missing ${budgetFile}, run with --update`)
  process.exit(1)
}

const ranked = [...sizes].sort(([, a], [, b]) => b - a)
console.log('[bundle-report] largest chunks (raw size / budget)')
for (const [name, size] of ranked.slice(0, TOP_N)) {
  const budget = budgets[name]
  console.log(
    `  ${name.padEnd(36)} ${toKb(size).padStart(10)} ${budget ? toKb(budget).padStart(10) : '    no budget'}`,
  )
}

let failed = false
for (const [name, size] of ranked) {
  const budget = budgets[name]
  if (budget !== undefined && size > budget) {
    failed = true
    console.error(
      `[bundle-report] FAIL ${name}: ${toKb(size)} exceeds its budget of ${toKb(budget)}`,
    )
  } else if (budget === undefined && size >= MIN_BUDGETED_BYTES) {
    console.warn(
      `[bundle-report] note: ${name} (${toKb(size)}) has no budget, run --update to add one`,
    )
  }
}

if (failed) {
  console.error(
    '[bundle-report] a chunk grew past its budget. Split it with dynamic import(), or raise its entry in bundle-budgets.json if the growth is intended.',
  )
  process.exit(1)
}
