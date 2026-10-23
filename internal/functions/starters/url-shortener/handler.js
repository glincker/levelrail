'use strict'
const crypto = require('node:crypto')

const SLUG = /^[A-Za-z0-9_-]{3,32}$/
const ALPHABET = 'abcdefghijkmnpqrstuvwxyz23456789'

function randomSlug(length = 7) {
  const bytes = crypto.randomBytes(length)
  return Array.from(bytes, (b) => ALPHABET[b % ALPHABET.length]).join('')
}

function safeEqual(a, b) {
  const x = Buffer.from(String(a))
  const y = Buffer.from(String(b))
  return x.length === y.length && crypto.timingSafeEqual(x, y)
}

function validUrl(raw) {
  try {
    const u = new URL(raw)
    return u.protocol === 'http:' || u.protocol === 'https:'
  } catch {
    return false
  }
}

function createHandler({ env, kv }) {
  const adminToken = env.ADMIN_TOKEN || ''
  const maxTTL = Number(env.MAX_TTL_SECONDS || 0)

  function authorized(headers) {
    const got = (headers.authorization || '').replace(/^Bearer\s+/i, '')
    return adminToken !== '' && safeEqual(got, adminToken)
  }

  return async function handle(req) {
    if (req.method === 'POST' && req.path === '/api/links') {
      if (!authorized(req.headers)) return { status: 401, body: { error: 'missing or wrong bearer token' } }
      let input
      try {
        input = JSON.parse(req.body.toString() || '{}')
      } catch {
        return { status: 400, body: { error: 'body must be JSON' } }
      }
      if (!validUrl(input.url)) return { status: 400, body: { error: 'url must be an http or https address' } }
      let slug = input.slug
      if (slug !== undefined && !SLUG.test(slug)) return { status: 400, body: { error: 'slug must be 3 to 32 letters, digits, - or _' } }
      if (slug === undefined) slug = randomSlug()
      else if ((await kv.get(`slug:${slug}`)) !== null) return { status: 409, body: { error: 'slug is taken' } }
      let ttl = Number(input.ttl_seconds || 0)
      if (maxTTL > 0 && (ttl <= 0 || ttl > maxTTL)) ttl = maxTTL
      await kv.putJSON(`slug:${slug}`, { url: input.url, hits: 0 }, { ttl })
      return { status: 201, body: { slug, path: `/${slug}` } }
    }

    if (req.method === 'GET' && SLUG.test(req.path.slice(1))) {
      const slug = req.path.slice(1)
      const link = await kv.getJSON(`slug:${slug}`)
      if (!link) return { status: 404, body: { error: 'unknown link' } }
      return { status: 302, headers: { location: link.url, 'cache-control': 'no-store' } }
    }

    if (req.method === 'GET' && req.path === '/') return { status: 200, body: { service: 'url-shortener' } }
    return { status: 404, body: { error: 'not found' } }
  }
}

module.exports = { createHandler, randomSlug }
