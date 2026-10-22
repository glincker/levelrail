'use strict'
const crypto = require('node:crypto')

function safeEqual(a, b) {
  const x = Buffer.from(String(a))
  const y = Buffer.from(String(b))
  return x.length === y.length && crypto.timingSafeEqual(x, y)
}

// One entry per chat service; a channel is used when its env vars are set.
function targets(env) {
  const out = []
  if (env.SLACK_WEBHOOK_URL) out.push({ name: 'slack', url: env.SLACK_WEBHOOK_URL, body: (text) => ({ text }) })
  if (env.DISCORD_WEBHOOK_URL) out.push({ name: 'discord', url: env.DISCORD_WEBHOOK_URL, body: (text) => ({ content: text.slice(0, 2000) }) })
  if (env.TELEGRAM_BOT_TOKEN && env.TELEGRAM_CHAT_ID) {
    out.push({
      name: 'telegram',
      url: `https://api.telegram.org/bot${env.TELEGRAM_BOT_TOKEN}/sendMessage`,
      body: (text) => ({ chat_id: env.TELEGRAM_CHAT_ID, text: text.slice(0, 4000) }),
    })
  }
  return out
}

function createHandler({ env, kv, fetch }) {
  const token = env.WEBHOOK_TOKEN || ''
  const dedupeTTL = Number(env.DEDUPE_TTL_SECONDS || 3600)

  return async function handle(req) {
    if (req.method !== 'POST' || req.path !== '/') return { status: 404, body: { error: 'not found' } }
    if (!token || !safeEqual(req.headers['x-webhook-token'] || '', token)) return { status: 401, body: { error: 'wrong webhook token' } }

    const channels = targets(env)
    if (channels.length === 0) return { status: 503, body: { error: 'no chat channel configured' } }

    let payload
    try {
      payload = JSON.parse(req.body.toString() || '{}')
    } catch {
      return { status: 400, body: { error: 'body must be JSON' } }
    }
    const text = String(payload.text || '').trim()
    if (!text) return { status: 400, body: { error: 'text is required' } }

    const deliveryId = req.headers['x-delivery-id']
    if (deliveryId) {
      const key = `delivery:${crypto.createHash('sha256').update(String(deliveryId)).digest('hex').slice(0, 32)}`
      if ((await kv.get(key)) !== null) return { status: 200, body: { ok: true, duplicate: true } }
      await kv.put(key, '1', { ttl: dedupeTTL })
    }

    const line = payload.user ? `${payload.user}: ${text}` : text
    const results = await Promise.all(
      channels.map(async (c) => {
        try {
          const res = await fetch(c.url, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(c.body(line)) })
          return { channel: c.name, ok: res.ok }
        } catch {
          return { channel: c.name, ok: false }
        }
      }),
    )
    return { status: results.some((r) => r.ok) ? 200 : 502, body: { ok: results.some((r) => r.ok), results } }
  }
}

module.exports = { createHandler, targets }
