// Run: node --test scripts/release-notes-wrap.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import { wrap, config, extractTheme, humans, stripBlocks } from './release-notes-wrap.mjs'

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
  assert.match(out, /<h2>Levelrail 1\.4\.0: deploy: one-click rollback from the status card<\/h2>/)
  for (const b of ['release-1.4.0', 'img.shields.io/github/license', 'docker/pulls', 'github/stars', 'img.shields.io/discord']) assert.ok(out.includes(b), b)
  for (const l of ['/installing', '/upgrade-safely', '/changelog', '/features', '/verify-a-release', 'compare/v1.3.2...v1.4.0', '/discussions']) assert.ok(out.includes(l), l)
})

test('image block lists real references', () => {
  assert.match(out, /docker pull ghcr\.io\/glincker\/levelrail:v1\.4\.0/)
  assert.match(out, /docker pull ghcr\.io\/glincker\/levelrail-agent:v1\.4\.0/)
  assert.match(out, /curl -fsSL https:\/\/levelrail\.com\/install\.sh/)
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
