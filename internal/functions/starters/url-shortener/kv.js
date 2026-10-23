'use strict'

// Client for the platform key-value store. KV_URL and KV_TOKEN are injected
// when the key-value store is enabled for the function.
function createKV(env = process.env, fetchImpl = globalThis.fetch) {
  const base = (env.KV_URL || '').replace(/\/+$/, '')
  const token = env.KV_TOKEN || ''

  async function call(method, path, params, body) {
    if (!base || !token) throw new Error('key-value store is not enabled: KV_URL and KV_TOKEN are missing')
    const url = `${base}${path}?${new URLSearchParams(params).toString()}`
    return fetchImpl(url, { method, headers: { authorization: `Bearer ${token}` }, body })
  }

  async function fail(res, what) {
    const text = await res.text().catch(() => '')
    throw new Error(`kv ${what} failed with ${res.status}${text ? `: ${text.slice(0, 200)}` : ''}`)
  }

  const kv = {
    async get(key) {
      const res = await call('GET', '/entry', { key })
      if (res.status === 404) return null
      if (!res.ok) await fail(res, 'get')
      return res.text()
    },
    async put(key, value, { ttl } = {}) {
      const params = { key }
      if (ttl) params.ttl = String(ttl)
      const res = await call('PUT', '/entry', params, value)
      if (!res.ok) await fail(res, 'put')
    },
    async delete(key) {
      const res = await call('DELETE', '/entry', { key })
      if (!res.ok && res.status !== 404) await fail(res, 'delete')
    },
    async list(prefix = '', limit = 100) {
      const res = await call('GET', '/keys', { prefix, limit: String(limit) })
      if (!res.ok) await fail(res, 'list')
      const out = await res.json()
      return out.keys.map((k) => k.key)
    },
    async getJSON(key) {
      const raw = await kv.get(key)
      return raw === null ? null : JSON.parse(raw)
    },
    async putJSON(key, value, opts) {
      await kv.put(key, JSON.stringify(value), opts)
    },
  }
  return kv
}

// In-memory stand-in with the same surface, used by the unit tests.
function createMemoryKV() {
  const data = new Map()
  const kv = {
    data,
    async get(key) { return data.has(key) ? data.get(key).value : null },
    async put(key, value, { ttl } = {}) { data.set(key, { value: String(value), ttl: ttl || 0 }) },
    async delete(key) { data.delete(key) },
    async list(prefix = '', limit = 100) {
      return [...data.keys()].filter((k) => k.startsWith(prefix)).sort().slice(0, limit)
    },
    async getJSON(key) { const r = await kv.get(key); return r === null ? null : JSON.parse(r) },
    async putJSON(key, value, opts) { await kv.put(key, JSON.stringify(value), opts) },
  }
  return kv
}

module.exports = { createKV, createMemoryKV }
