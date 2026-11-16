// Run: node --test scripts/gen-versions-table.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { parseReleases, renderTable, splice, BEGIN, END } from './gen-versions-table.mjs'

const cl = `# Changelog

## [1.3.2](https://x/compare/v1.3.1...v1.3.2) (2026-11-16)

### Features

## [0.2.0-beta](https://x/compare/v0.1.0...v0.2.0-beta) (2026-09-12)
`

test('parses version and date in order', () => {
  assert.deepEqual(parseReleases(cl), [
    { version: '1.3.2', date: '2026-11-16' },
    { version: '0.2.0-beta', date: '2026-09-12' },
  ])
})

test('marks latest and pre-release, links the tag', () => {
  const t = renderTable(parseReleases(cl), 'o/r')
  assert.match(t, /\| 1\.3\.2 \| 2026-11-16 \| Latest \| \[Release notes\]\(https:\/\/github\.com\/o\/r\/releases\/tag\/v1\.3\.2\) \|/)
  assert.match(t, /\| 0\.2\.0-beta \| 2026-09-12 \| Pre-release \|/)
})

test('splice replaces only the marked block and is idempotent', () => {
  const doc = `a\n${BEGIN}\nold\n${END}\nb\n`
  const once = splice(doc, 'T')
  assert.equal(once, `a\n${BEGIN}\nT\n${END}\nb\n`)
  assert.equal(splice(once, 'T'), once)
})
