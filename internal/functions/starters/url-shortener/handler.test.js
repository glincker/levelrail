'use strict'
const test = require('node:test')
const assert = require('node:assert/strict')
const { createHandler, randomSlug } = require('./handler')
const { createMemoryKV } = require('./kv')

function setup(env = {}) {
  const kv = createMemoryKV()
  const handle = createHandler({ env: { ADMIN_TOKEN: 'admin-secret', ...env }, kv })
  const post = (body, token = 'admin-secret') =>
    handle({ method: 'POST', path: '/api/links', headers: token ? { authorization: `Bearer ${token}` } : {}, body: Buffer.from(JSON.stringify(body)) })
  const get = (path) => handle({ method: 'GET', path, headers: {}, body: Buffer.alloc(0) })
  return { kv, post, get }
}

test('creates a link and redirects to it', async () => {
  const { post, get } = setup()
  const created = await post({ url: 'https://example.com/a?b=1', slug: 'docs' })
  assert.equal(created.status, 201)
  const hit = await get('/docs')
  assert.equal(hit.status, 302)
  assert.equal(hit.headers.location, 'https://example.com/a?b=1')
})

test('generates a slug when none is given', async () => {
  const { post, kv } = setup()
  const res = await post({ url: 'https://example.com' })
  assert.equal(res.status, 201)
  assert.match(res.body.slug, /^[a-z2-9]{7}$/)
  assert.ok(await kv.get(`slug:${res.body.slug}`))
})

test('rejects missing or wrong token', async () => {
  const { post } = setup()
  assert.equal((await post({ url: 'https://example.com' }, '')).status, 401)
  assert.equal((await post({ url: 'https://example.com' }, 'nope')).status, 401)
})

test('refuses creation when ADMIN_TOKEN is unset', async () => {
  const { post } = setup({ ADMIN_TOKEN: '' })
  assert.equal((await post({ url: 'https://example.com' }, '')).status, 401)
})

test('validates url and slug', async () => {
  const { post } = setup()
  assert.equal((await post({ url: 'javascript:alert(1)' })).status, 400)
  assert.equal((await post({ url: 'not a url' })).status, 400)
  assert.equal((await post({ url: 'https://example.com', slug: 'a b' })).status, 400)
})

test('a taken slug returns 409', async () => {
  const { post } = setup()
  await post({ url: 'https://example.com', slug: 'dup' })
  assert.equal((await post({ url: 'https://example.org', slug: 'dup' })).status, 409)
})

test('unknown slug is 404', async () => {
  const { get } = setup()
  assert.equal((await get('/missing')).status, 404)
})

test('MAX_TTL_SECONDS caps the lifetime', async () => {
  const { post, kv } = setup({ MAX_TTL_SECONDS: '60' })
  await post({ url: 'https://example.com', slug: 'short', ttl_seconds: 99999 })
  assert.equal(kv.data.get('slug:short').ttl, 60)
})

test('randomSlug uses the requested length', () => {
  assert.equal(randomSlug(10).length, 10)
})
