#!/usr/bin/env node
import { readdirSync, statSync } from 'node:fs'
import path from 'node:path'

const distAssets = path.resolve(import.meta.dirname, '..', 'dist', 'assets')

// Locks in the current main chunk size (~556.5 kB as of 2026-08-31) as a
// ceiling with headroom, not a target: fulfills the per-chunk assertion
// vite.config.ts's visualizer comment calls deferred.
const DEFAULT_BUDGET_BYTES = 600_000
const budgetBytes = Number(process.env.BUNDLE_SIZE_BUDGET_BYTES) || DEFAULT_BUDGET_BYTES

function toKb(bytes) {
  return (bytes / 1000).toFixed(1)
}

let entries
try {
  entries = readdirSync(distAssets)
} catch {
  console.error(`[check-bundle-size] no build output at ${distAssets}, run \`npm run build\` first`)
  process.exit(1)
}

const jsFiles = entries.filter((name) => name.endsWith('.js')).sort()
if (jsFiles.length === 0) {
  console.error(`[check-bundle-size] no .js chunks found in ${distAssets}`)
  process.exit(1)
}

let failed = false
for (const name of jsFiles) {
  const size = statSync(path.join(distAssets, name)).size
  if (size > budgetBytes) {
    failed = true
    console.error(
      `[check-bundle-size] FAIL ${name}: ${toKb(size)} kB exceeds budget of ${toKb(budgetBytes)} kB`,
    )
  }
}

if (failed) {
  console.error(
    '[check-bundle-size] one or more chunks exceed the size budget. See CLAUDE.md section 7 (route-level code splitting) and web/vite.config.ts.',
  )
  process.exit(1)
}

console.log(`[check-bundle-size] all ${jsFiles.length} chunks within ${toKb(budgetBytes)} kB budget`)
