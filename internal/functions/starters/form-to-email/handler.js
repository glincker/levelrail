'use strict'
const crypto = require('node:crypto')

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const MAX_MESSAGE = 5000

function safeEqual(a, b) {
  const x = Buffer.from(String(a))
  const y = Buffer.from(String(b))
  return x.length === y.length && crypto.timingSafeEqual(x, y)
}

function parseForm(req) {
  const type = req.headers['content-type'] || ''
  const text = req.body.toString('utf8')
  if (type.includes('application/json')) return JSON.parse(text || '{}')
  return Object.fromEntries(new URLSearchParams(text))
}

// The platform has no function-facing send API, so submissions are stored in
// the key-value store and optionally forwarded to NOTIFY_WEBHOOK_URL (a chat
// or mail-bridge webhook that accepts {"text": "..."}).
function createHandler({ env, kv, fetch, now = Date.now }) {
  const adminToken = env.ADMIN_TOKEN || ''
  const notifyURL = env.NOTIFY_WEBHOOK_URL || ''
  const ttl = Number(env.SUBMISSION_TTL_SECONDS || 30 * 24 * 3600)
  const origin = env.ALLOWED_ORIGIN || '*'
  const cors = { 'access-control-allow-origin': origin, 'access-control-allow-headers': 'content-type' }

  return async function handle(req) {
    if (req.method === 'OPTIONS') return { status: 204, headers: cors }

    if (req.method === 'POST' && req.path === '/') {
      let form
      try {
        form = parseForm(req)
      } catch {
        return { status: 400, headers: cors, body: { error: 'could not read the form body' } }
      }
      if (form.website) return { status: 200, headers: cors, body: { ok: true } }
      const name = String(form.name || '').trim().slice(0, 200)
      const email = String(form.email || '').trim()
      const message = String(form.message || '').trim()
      if (!EMAIL.test(email)) return { status: 400, headers: cors, body: { error: 'a valid email is required' } }
      if (!message || message.length > MAX_MESSAGE) return { status: 400, headers: cors, body: { error: `message is required, up to ${MAX_MESSAGE} characters` } }

      const at = now()
      const id = `${String(at).padStart(15, '0')}-${crypto.randomBytes(4).toString('hex')}`
      await kv.putJSON(`submission:${id}`, { id, name, email, message, received_at: new Date(at).toISOString() }, { ttl })
      let notified = false
      if (notifyURL) {
        try {
          const res = await fetch(notifyURL, {
            method: 'POST',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ text: `New message from ${name || 'someone'} <${email}>: ${message}` }),
          })
          notified = res.ok
        } catch {
          notified = false
        }
      }
      return { status: 201, headers: cors, body: { ok: true, id, notified } }
    }

    if (req.method === 'GET' && req.path === '/submissions') {
      const got = (req.headers.authorization || '').replace(/^Bearer\s+/i, '')
      if (!adminToken || !safeEqual(got, adminToken)) return { status: 401, body: { error: 'missing or wrong bearer token' } }
      const keys = (await kv.list('submission:', 1000)).sort().reverse().slice(0, 50)
      const submissions = []
      for (const key of keys) {
        const s = await kv.getJSON(key)
        if (s) submissions.push(s)
      }
      return { status: 200, body: { submissions } }
    }

    return { status: 404, body: { error: 'not found' } }
  }
}

module.exports = { createHandler }
