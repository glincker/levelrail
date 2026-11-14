import test from 'node:test'
import assert from 'node:assert/strict'
import { check, parseCommit } from './check-commit-semver.mjs'

const A = (path) => ({ status: 'A', path })
const M = (path) => ({ status: 'M', path })
const D = (path) => ({ status: 'D', path })

test('parseCommit reads bang and footer', () => {
  assert.equal(parseCommit('feat!: x').bang, true)
  assert.equal(parseCommit('fix: x\n\nBREAKING CHANGE: gone').footer, true)
  assert.equal(parseCommit('nonsense').type, null)
})

test('feat with a user-visible path passes', () => {
  assert.deepEqual(check('feat: add flag', [M('cmd/levelrail/deploy.go')]), [])
})

test('feat touching only docs and tests fails', () => {
  assert.equal(check('feat: add thing', [M('docs/a.mdx'), M('internal/x/x_test.go')]).length, 1)
})

test('fix that adds a new api file fails', () => {
  assert.equal(check('fix: tweak', [A('internal/api/new_route.go')]).length, 1)
})

test('removing stable surface without bang fails', () => {
  assert.equal(check('fix: drop', [D('docs/schemas/resources.schema.json')]).length, 1)
})

test('removal with bang and footer passes', () => {
  assert.deepEqual(check('feat!: drop\n\nBREAKING CHANGE: removed', [D('internal/iac/schema/old.schema.json')]), [])
})

test('bang without footer or surface change fails', () => {
  assert.equal(check('feat!: big', [M('cmd/a.go')]).length, 2)
})

test('chore is never flagged for plain edits', () => {
  assert.deepEqual(check('chore: tidy', [M('README.md')]), [])
})
