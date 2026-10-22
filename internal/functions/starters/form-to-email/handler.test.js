'use strict'
const test = require('node:test')
const assert = require('node:assert/strict')
const { createHandler } = require('./handler')
const { createMemoryKV } = require('./kv')

function setup(env = {}, fetchImpl) {
  const kv = createMemoryKV()
  const calls = []
  const fetch = fetchImpl || (async (url, init) => { calls.push({ url, init }); return { ok: true } })
  const handle = createHandler({ env: { ADMIN_TOKEN: 'adm', ...env }, kv, fetch, now: () => 1_700_000_000_000 })
  return { kv, calls, handle }
}

const json = (body) => ({ method: 'POST', path: '/', headers: { 'content-type': 'application/json' }, body: Buffer.from(JSON.stringify(body)) })

test('stores a valid JSON submission', async () => {
  const { handle, kv } = setup()
  const res = await handle(json({ name: 'Ada', email: 'ada@example.com', message: 'hello' }))
  assert.equal(res.status, 201)
  const saved = await kv.getJSON(`submission:${res.body.id}`)
  assert.equal(saved.email, 'ada@example.com')
  assert.equal(res.body.notified, false)
})

test('reads url-encoded forms', async () => {
  const { handle } = setup()
  const res = await handle({ method: 'POST', path: '/', headers: { 'content-type': 'application/x-www-form-urlencoded' }, body: Buffer.from('email=a%40b.co&message=hi') })
  assert.equal(res.status, 201)
})

test('validates email and message', async () => {
  const { handle } = setup()
  assert.equal((await handle(json({ email: 'nope', message: 'x' }))).status, 400)
  assert.equal((await handle(json({ email: 'a@b.co', message: '' }))).status, 400)
  assert.equal((await handle(json({ email: 'a@b.co', message: 'x'.repeat(5001) }))).status, 400)
})

test('honeypot field is accepted silently and stores nothing', async () => {
  const { handle, kv } = setup()
  const res = await handle(json({ email: 'a@b.co', message: 'x', website: 'spam.example' }))
  assert.equal(res.status, 200)
  assert.equal(kv.data.size, 0)
})

test('forwards to NOTIFY_WEBHOOK_URL when set', async () => {
  const { handle, calls } = setup({ NOTIFY_WEBHOOK_URL: 'https://hooks.example/x' })
  const res = await handle(json({ name: 'Ada', email: 'a@b.co', message: 'yo' }))
  assert.equal(res.body.notified, true)
  assert.equal(calls.length, 1)
  assert.match(JSON.parse(calls[0].init.body).text, /Ada/)
})

test('a failing notify hook does not lose the submission', async () => {
  const { handle, kv } = setup({ NOTIFY_WEBHOOK_URL: 'https://hooks.example/x' }, async () => { throw new Error('down') })
  const res = await handle(json({ email: 'a@b.co', message: 'yo' }))
  assert.equal(res.status, 201)
  assert.equal(res.body.notified, false)
  assert.equal(kv.data.size, 1)
})

test('submissions list needs the admin token', async () => {
  const { handle } = setup()
  await handle(json({ email: 'a@b.co', message: 'yo' }))
  const get = (headers) => handle({ method: 'GET', path: '/submissions', headers, body: Buffer.alloc(0) })
  assert.equal((await get({})).status, 401)
  assert.equal((await get({ authorization: 'Bearer adm' })).body.submissions.length, 1)
})
