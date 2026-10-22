'use strict'
const crypto = require('node:crypto')

function safeEqual(a, b) {
  const x = Buffer.from(String(a))
  const y = Buffer.from(String(b))
  return x.length === y.length && crypto.timingSafeEqual(x, y)
}

// Signature is sha256=hex(HMAC(secret, body)), or with X-Signature-Timestamp
// sha256=hex(HMAC(secret, timestamp + "." + body)) and a replay window.
function verifySignature({ secret, headers, body, now, windowSeconds }) {
  const sig = headers['x-signature'] || headers['x-signature-256'] || ''
  if (!secret || !sig) return false
  const ts = headers['x-signature-timestamp']
  let signed = body
  if (ts !== undefined) {
    const sec = Number(ts)
    if (!Number.isFinite(sec) || Math.abs(now / 1000 - sec) > windowSeconds) return false
    signed = Buffer.concat([Buffer.from(`${ts}.`), body])
  }
  const want = `sha256=${crypto.createHmac('sha256', secret).update(signed).digest('hex')}`
  return safeEqual(sig.trim(), want)
}

function createHandler({ env, kv, now = Date.now }) {
  const secret = env.WEBHOOK_SECRET || ''
  const adminToken = env.ADMIN_TOKEN || ''
  const ttl = Number(env.EVENT_TTL_SECONDS || 7 * 24 * 3600)
  const windowSeconds = Number(env.SIGNATURE_WINDOW_SECONDS || 300)

  return async function handle(req) {
    if (req.method === 'POST' && req.path === '/') {
      if (!verifySignature({ secret, headers: req.headers, body: req.body, now: now(), windowSeconds })) {
        return { status: 401, body: { error: 'invalid signature' } }
      }
      const at = now()
      const id = `${String(at).padStart(15, '0')}-${crypto.randomBytes(4).toString('hex')}`
      const event = {
        id,
        received_at: new Date(at).toISOString(),
        content_type: req.headers['content-type'] || '',
        body: req.body.toString('utf8'),
      }
      await kv.putJSON(`event:${id}`, event, { ttl })
      return { status: 202, body: { id } }
    }

    if (req.method === 'GET' && req.path === '/events') {
      const got = (req.headers.authorization || '').replace(/^Bearer\s+/i, '')
      if (!adminToken || !safeEqual(got, adminToken)) return { status: 401, body: { error: 'missing or wrong bearer token' } }
      const limit = Math.min(Number(req.query?.get('limit') || 20), 100)
      const keys = (await kv.list('event:', 1000)).sort().reverse().slice(0, limit)
      const events = []
      for (const key of keys) {
        const ev = await kv.getJSON(key)
        if (ev) events.push(ev)
      }
      return { status: 200, body: { events } }
    }

    return { status: 404, body: { error: 'not found' } }
  }
}

module.exports = { createHandler, verifySignature }
