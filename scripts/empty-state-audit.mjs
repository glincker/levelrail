#!/usr/bin/env node
// Lists main pages whose own code and local imports never render an EmptyState.
// Usage: node scripts/empty-state-audit.mjs [--strict]
import { readFileSync, existsSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'

const root = resolve(new URL('..', import.meta.url).pathname, 'web/src')
const pages = [
  'apps/index',
  'domains',
  'databases',
  'backups',
  'nodes',
  'alerts',
  'dns/index',
  'builds',
  'images',
  'deployments',
  'functions',
  'dashboards',
  'firewall/index',
  'settings/storage',
  'settings/backup-targets',
  'settings/notification-channels',
  'settings/registry-credentials',
  'settings/tokens',
  'settings/users',
]
const marker = /EmptyState|empty-state|emptyTitle|[Ee]mpty[A-Z]\w*Title/

function resolveImport(spec, from) {
  let base
  if (spec.startsWith('@/')) base = join(root, spec.slice(2))
  else if (spec.startsWith('.')) base = join(dirname(from), spec)
  else return null
  for (const ext of ['.tsx', '.ts', '/index.tsx']) {
    if (existsSync(base + ext) && statSync(base + ext).isFile()) return base + ext
  }
  return null
}

function hasEmptyState(file, depth) {
  const src = readFileSync(file, 'utf8')
  if (marker.test(src)) return true
  if (depth === 0) return false
  for (const m of src.matchAll(/from\s+'((?:@\/|\.)[^']*)'/g)) {
    const target = resolveImport(m[1], file)
    if (target && !target.includes('/queries/') && hasEmptyState(target, depth - 1)) return true
  }
  return false
}

const missing = []
let checked = 0
for (const page of pages) {
  const candidates = ['.tsx', '/index.tsx'].map((e) => join(root, 'routes', page + e))
  const file = candidates.find(existsSync)
  if (!file) continue
  checked++
  const ok = hasEmptyState(file, 4)
  console.log(`${ok ? 'ok     ' : 'MISSING'} ${page}`)
  if (!ok) missing.push(page)
}
console.log(`\n${missing.length} of ${checked} pages have no empty state`)
if (missing.length && process.argv.includes('--strict')) process.exit(1)
