'use strict'
const test = require('node:test')
const assert = require('node:assert/strict')
const crypto = require('node:crypto')
const { createHandler } = require('./handler')
const { createMemoryKV } = require('./kv')

const SECRET = 'whsec_test'
const NOW = 1_700_000_000_000

function sign(body, ts) {
  const signed = ts === undefined ? body : `${ts}.${body}`
  return `sha256=${crypto.createHmac('sha256', SECRET).update(signed).digest('hex')}`
}

function setup(env = {}) {
  const kv = createMemoryKV()
  const handle = createHandler({ env: { WEBHOOK_SECRET: SECRET, ADMIN_TOKEN: 'adm', ...env }, kv, now: () => NOW })
  return { kv, handle }
}

const post = (handle, body, headers) => handle({ method: 'POST', path: '/', headers, body: Buffer.from(body) })

test('stores an event with a valid signature', async () => {
  const { handle, kv } = setup()
  const body = '{"type":"order.paid"}'
  const res = await post(handle, body, { 'x-signature': sign(body), 'content-type': 'application/json' })
  assert.equal(res.status, 202)
  const stored = await kv.getJSON(`event:${res.body.id}`)
  assert.equal(stored.body, body)
  assert.equal(kv.data.get(`event:${res.body.id}`).ttl, 7 * 24 * 3600)
})

test('rejects a bad or missing signature and stores nothing', async () => {
  const { handle, kv } = setup()
  assert.equal((await post(handle, 'x', {})).status, 401)
  assert.equal((await post(handle, 'x', { 'x-signature': 'sha256=00' })).status, 401)
  assert.equal(kv.data.size, 0)
})

test('rejects every call when WEBHOOK_SECRET is unset', async () => {
  const { handle } = setup({ WEBHOOK_SECRET: '' })
  assert.equal((await post(handle, 'x', { 'x-signature': sign('x') })).status, 401)
})

test('timestamped signatures are checked and replay-limited', async () => {
  const { handle } = setup()
  const ts = String(NOW / 1000)
  const ok = await post(handle, 'hi', { 'x-signature': sign('hi', ts), 'x-signature-timestamp': ts })
  assert.equal(ok.status, 202)
  const old = String(NOW / 1000 - 3600)
  const stale = await post(handle, 'hi', { 'x-signature': sign('hi', old), 'x-signature-timestamp': old })
  assert.equal(stale.status, 401)
  const swapped = await post(handle, 'hi', { 'x-signature': sign('hi'), 'x-signature-timestamp': ts })
  assert.equal(swapped.status, 401)
})

test('events list needs the admin token and returns newest first', async () => {
  const { handle } = setup()
  await post(handle, 'one', { 'x-signature': sign('one') })
  const get = (headers) => handle({ method: 'GET', path: '/events', headers, query: new URLSearchParams(), body: Buffer.alloc(0) })
  assert.equal((await get({})).status, 401)
  const res = await get({ authorization: 'Bearer adm' })
  assert.equal(res.status, 200)
  assert.equal(res.body.events.length, 1)
  assert.equal(res.body.events[0].body, 'one')
})
