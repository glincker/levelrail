'use strict'
const test = require('node:test')
const assert = require('node:assert/strict')
const { createHandler, jobs } = require('./handler')
const { createMemoryKV } = require('./kv')

function setup(env = {}, registry = jobs, fetchImpl = async () => ({ ok: true })) {
  const kv = createMemoryKV()
  let t = 1_700_000_000_000
  const handle = createHandler({ env: { CRON_SECRET: 's3cret', ...env }, kv, fetch: fetchImpl, now: () => (t += 10), registry })
  const run = (job, secret = 's3cret') => handle({ method: 'POST', path: `/cron/${secret}/${job}`, headers: {}, body: Buffer.alloc(0) })
  return { kv, handle, run }
}

test('runs a job, records the run and releases the lock', async () => {
  const { run, kv } = setup({ HEARTBEAT_URL: 'https://ping.example/x' })
  const res = await run('heartbeat')
  assert.equal(res.status, 200)
  assert.equal(res.body.summary, 'pinged')
  assert.equal(await kv.get('lock:heartbeat'), null)
  assert.equal((await kv.list('run:')).length, 1)
})

test('a wrong or missing secret is refused', async () => {
  assert.equal((await setup().run('heartbeat', 'bad')).status, 401)
  assert.equal((await setup({ CRON_SECRET: '' }).run('heartbeat', '')).status, 401)
})

test('unknown job is 404', async () => {
  assert.equal((await setup().run('nope')).status, 404)
})

test('a failing job reports 500, is recorded and frees the lock', async () => {
  const { run, kv } = setup({}, { boom: async () => { throw new Error('kaput') } })
  const res = await run('boom')
  assert.equal(res.status, 500)
  assert.equal(res.body.error, 'kaput')
  assert.equal(await kv.get('lock:boom'), null)
})

test('an existing lock blocks an overlapping run', async () => {
  const { run, kv } = setup()
  await kv.put('lock:heartbeat', '1')
  assert.equal((await run('heartbeat')).status, 409)
})

test('lock carries the configured ttl while the job runs', async () => {
  let seen
  const kvRef = {}
  const { run, kv } = setup({ JOB_LOCK_SECONDS: '42' }, { peek: async ({ kv: k }) => { seen = k.data.get('lock:peek').ttl } })
  kvRef.kv = kv
  await run('peek')
  assert.equal(seen, 42)
})

test('prune-runs keeps the newest 50 records', async () => {
  const { run, kv } = setup()
  for (let i = 0; i < 60; i++) await kv.put(`run:${String(i).padStart(15, '0')}:x`, '{}')
  const res = await run('prune-runs')
  assert.equal(res.body.summary, 'removed 10 old run records')
})

test('GET /runs lists recent runs', async () => {
  const { run, handle } = setup()
  await run('heartbeat')
  const res = await handle({ method: 'GET', path: '/runs', headers: {}, body: Buffer.alloc(0) })
  assert.equal(res.body.runs.length, 1)
})
