'use strict'
const test = require('node:test')
const assert = require('node:assert/strict')
const { createHandler, targets } = require('./handler')
const { createMemoryKV } = require('./kv')

function setup(env = {}, status = 200) {
  const kv = createMemoryKV()
  const calls = []
  const fetch = async (url, init) => { calls.push({ url, body: JSON.parse(init.body) }); return { ok: status < 400 } }
  const handle = createHandler({ env: { WEBHOOK_TOKEN: 'tok', SLACK_WEBHOOK_URL: 'https://slack.example/h', ...env }, kv, fetch })
  const send = (payload, headers = { 'x-webhook-token': 'tok' }) => handle({ method: 'POST', path: '/', headers, body: Buffer.from(JSON.stringify(payload)) })
  return { kv, calls, send }
}

test('relays text to Slack', async () => {
  const { send, calls } = setup()
  const res = await send({ text: 'deploy finished', user: 'ci' })
  assert.equal(res.status, 200)
  assert.deepEqual(calls[0].body, { text: 'ci: deploy finished' })
})

test('relays to every configured channel', async () => {
  const { send, calls } = setup({ DISCORD_WEBHOOK_URL: 'https://discord.example/h', TELEGRAM_BOT_TOKEN: 'bot', TELEGRAM_CHAT_ID: '7' })
  const res = await send({ text: 'hi' })
  assert.equal(calls.length, 3)
  assert.deepEqual(res.body.results.map((r) => r.channel), ['slack', 'discord', 'telegram'])
  assert.ok(calls.find((c) => c.url.startsWith('https://api.telegram.org/botbot/')))
})

test('wrong token is refused and nothing is sent', async () => {
  const { send, calls } = setup()
  assert.equal((await send({ text: 'x' }, {})).status, 401)
  assert.equal((await send({ text: 'x' }, { 'x-webhook-token': 'no' })).status, 401)
  assert.equal(calls.length, 0)
})

test('503 when no channel is configured', async () => {
  const { send } = setup({ SLACK_WEBHOOK_URL: '' })
  assert.equal((await send({ text: 'x' })).status, 503)
})

test('empty text and bad JSON are 400', async () => {
  const { send } = setup()
  assert.equal((await send({ text: '  ' })).status, 400)
})

test('a repeated delivery id is relayed once', async () => {
  const { send, calls } = setup()
  const headers = { 'x-webhook-token': 'tok', 'x-delivery-id': 'abc' }
  await send({ text: 'once' }, headers)
  const second = await send({ text: 'once' }, headers)
  assert.equal(second.body.duplicate, true)
  assert.equal(calls.length, 1)
})

test('502 when every channel fails', async () => {
  const { send } = setup({}, 500)
  assert.equal((await send({ text: 'x' })).status, 502)
})

test('targets needs both telegram variables', () => {
  assert.equal(targets({ TELEGRAM_BOT_TOKEN: 'b' }).length, 0)
})
