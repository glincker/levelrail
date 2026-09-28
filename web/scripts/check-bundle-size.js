#!/usr/bin/env node
import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'

const distDir = path.resolve(import.meta.dirname, '..', 'dist')
const distAssets = path.join(distDir, 'assets')

// Locks in the current main chunk size (~556.5 kB as of 2026-08-31) as a
// ceiling with headroom, not a target: fulfills the per-chunk assertion
// vite.config.ts's visualizer comment calls deferred.
const DEFAULT_BUDGET_BYTES = 600_000
const budgetBytes = Number(process.env.BUNDLE_SIZE_BUDGET_BYTES) || DEFAULT_BUDGET_BYTES

function toKb(bytes) {
  return (bytes / 1000).toFixed(1)
}

// mermaid ships every diagram type (architecture, mindmap, gitGraph, ...)
// as its own lazily-fetched chunk, several of them legitimately large
// (elk.js's layout engine, cytoscape, the shared chevrotain/langium-based
// parser core): none of that is ever downloaded unless a doc page actually
// uses that diagram type, so the general budget above doesn't apply to it.
// Identified by source module path via the build manifest (vite.config.ts
// only writes one when BUNDLE_MANIFEST=true, which CI's web-check job
// sets) rather than by output filename, since content-hash suffixes and
// Rollup's anonymous "chunk-XXXXXXXX" names for shared chunks change every
// build and aren't otherwise distinguishable from genuine app chunks.
const EXEMPT_MODULE_RE =
  /^node_modules\/(mermaid|@mermaid-js\/parser|elkjs|cytoscape[^/]*|chevrotain[^/]*|@chevrotain\/|langium|dagre-d3-es|khroma|cose-bilkent|@upsetjs\/venn\.js)\//

function loadExemptFiles() {
  let manifest
  try {
    manifest = JSON.parse(readFileSync(path.join(distDir, '.vite', 'manifest.json'), 'utf8'))
  } catch {
    return new Set()
  }

  const keys = Object.keys(manifest)
  const exemptKeys = new Set(keys.filter((key) => EXEMPT_MODULE_RE.test(key)))

  // Some of mermaid's own shared/common chunks (e.g. its chevrotain-based
  // parser core) have no module path of their own in the manifest, only
  // an anonymous "_chunk-XXXX.js" key; a chunk like that is still exempt
  // when every entry that imports it is already exempt, i.e. it's pure
  // shared infrastructure reachable only from mermaid's own tree.
  let changed = true
  while (changed) {
    changed = false
    for (const key of keys) {
      if (exemptKeys.has(key)) continue
      const importers = keys.filter((k) => {
        const entry = manifest[k]
        return (entry.imports || []).includes(key) || (entry.dynamicImports || []).includes(key)
      })
      if (importers.length > 0 && importers.every((k) => exemptKeys.has(k))) {
        exemptKeys.add(key)
        changed = true
      }
    }
  }

  const files = new Set()
  for (const key of exemptKeys) {
    const file = manifest[key]?.file
    if (file) files.add(path.basename(file))
  }
  return files
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

const exemptFiles = loadExemptFiles()
let failed = false
let checked = 0
for (const name of jsFiles) {
  if (exemptFiles.has(name)) continue
  checked++
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
    '[check-bundle-size] one or more chunks exceed the size budget. Split large routes with dynamic import() and see web/vite.config.ts.',
  )
  process.exit(1)
}

const exemptNote = exemptFiles.size > 0 ? ` (${exemptFiles.size} exempt lazy dependency chunks skipped)` : ''
console.log(`[check-bundle-size] all ${checked} chunks within ${toKb(budgetBytes)} kB budget${exemptNote}`)
