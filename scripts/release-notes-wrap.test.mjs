// Run: node --test scripts/release-notes-wrap.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { wrap, config, extractTheme, humans, stripBlocks, dedupeBody } from './release-notes-wrap.mjs'

const EM = String.fromCharCode(8212)
const DASHES = new RegExp(`[${String.fromCharCode(8211)}${EM}]`)

const body = `## What's new

### Features

* **deploy:** one-click rollback from the status card ([abc1234](https://x/commit/abc1234))

### Bug Fixes

* fix a thing
`

const people = ['alice', 'dependabot[bot]', 'release-please[bot]', 'github-actions[bot]', 'bob', 'alice', '']
const out = wrap({ version: '1.4.0', prev: 'v1.3.2', body, contributors: people }, config())

test('header has logo, title, badges, links', () => {
  assert.match(out, /raw\.githubusercontent\.com\/glincker\/levelrail\/main\/docs\/assets\/brand\/png\/mark-1024\.png/)
  assert.ok(out.includes('Levelrail, a lightweight, self-hosted deployment platform: git push to production on your own servers and cloud accounts.'))
  assert.match(out, /<h2>Levelrail 1\.4\.0: deploy: one-click rollback from the status card<\/h2>/)
  for (const b of ['release-1.4.0', 'img.shields.io/github/license', 'docker/pulls', 'github/stars', 'img.shields.io/discord']) assert.ok(out.includes(b), b)
  for (const l of ['/installing', '/upgrade-safely', '/changelog', '/features', '/verify-a-release', 'docs.levelrail.com/faq', 'compare/v1.3.2...v1.4.0', '/discussions']) assert.ok(out.includes(l), l)
})

test('image block lists real references', () => {
  assert.match(out, /docker pull ghcr\.io\/glincker\/levelrail:v1\.4\.0/)
  assert.match(out, /docker pull ghcr\.io\/glincker\/levelrail-agent:v1\.4\.0/)
  assert.match(out, /curl -fsSL https:\/\/github\.com\/glincker\/levelrail\/releases\/latest\/download\/install\.sh/)
})

test('body kept and avatars skip bots and duplicates', () => {
  assert.ok(out.includes('* fix a thing'))
  assert.equal((out.match(/github\.com\/alice\.png\?size=48/g) || []).length, 1)
  assert.ok(out.includes('<a href="https://github.com/bob"><img src="https://github.com/bob.png?size=48" width="48" height="48" alt="@bob"></a>'))
  const row = out.split('Contributors to this release')[1]
  assert.ok(!/dependabot|release-please|github-actions/.test(row))
})

test('no dash characters, idempotent re-wrap', () => {
  assert.ok(!DASHES.test(out))
  assert.equal(wrap({ version: '1.4.0', prev: 'v1.3.2', body: out, contributors: people }, config()), out)
})

test('helpers', () => {
  assert.equal(extractTheme(`### Features\n\n* a ${EM} b ([#7](u)) by @x`), 'a, b')
  assert.deepEqual(humans(['a', 'x[bot]'], '\\[bot\\]$'), ['a'])
  assert.equal(stripBlocks('x'), 'x')
})

test('owner row when no humans resolve', () => {
  assert.match(wrap({ version: '1.4.0', prev: '', body, contributors: [] }, config()), /href="https:\/\/github\.com\/glincker">@glincker/)
})

const genBody = `${body}
## Contributors

- @carol made their first contribution in [#9](https://x/pull/9)

<a href="https://github.com/carol"><img src="https://github.com/carol.png?size=64" alt="@carol"></a> 

Thanks to @carol.

**Full changelog:** [v1.3.2...v1.4.0](https://github.com/glincker/levelrail/compare/v1.3.2...v1.4.0) | [Release page](https://docs.levelrail.com/changelog) | [Installing](https://docs.levelrail.com/installing)

Full Changelog: https://github.com/glincker/levelrail/compare/v1.3.2...v1.4.0
`

test('generator contributors and compare lines are removed, one of each remains', () => {
  const o = wrap({ version: '1.4.0', prev: 'v1.3.2', body: genBody, contributors: ['bob'] }, config())
  assert.equal((o.match(/Contributors/g) || []).length, 1)
  assert.equal((o.match(/Full Changelog/gi) || []).length, 1)
  assert.equal((o.match(/compare\/v1\.3\.2\.\.\.v1\.4\.0/g) || []).length, 2)
  assert.ok(!o.includes('carol'))
  assert.ok(o.includes('[Release page](https://docs.levelrail.com/changelog) | [Installing]'))
  assert.ok(o.includes('* fix a thing'))
  assert.equal(wrap({ version: '1.4.0', prev: 'v1.3.2', body: o, contributors: ['bob'] }, config()), o)
  assert.equal(dedupeBody('a\n\nb'), 'a\n\nb')
})
