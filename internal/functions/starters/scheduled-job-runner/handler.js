'use strict'
const crypto = require('node:crypto')

function safeEqual(a, b) {
  const x = Buffer.from(String(a))
  const y = Buffer.from(String(b))
  return x.length === y.length && crypto.timingSafeEqual(x, y)
}

// Add your jobs here. Each gets { kv, env, fetch, now } and may return a short summary.
const jobs = {
  async heartbeat({ env, fetch }) {
    if (!env.HEARTBEAT_URL) return 'HEARTBEAT_URL not set, nothing pinged'
    const res = await fetch(env.HEARTBEAT_URL, { method: 'GET' })
    if (!res.ok) throw new Error(`heartbeat answered ${res.status}`)
    return 'pinged'
  },
  async 'prune-runs'({ kv }) {
    const keys = await kv.list('run:', 1000)
    const stale = keys.sort().slice(0, Math.max(0, keys.length - 50))
    for (const key of stale) await kv.delete(key)
    return `removed ${stale.length} old run records`
  },
}

function createHandler({ env, kv, fetch, now = Date.now, registry = jobs }) {
  const secret = env.CRON_SECRET || ''
  const lockSeconds = Number(env.JOB_LOCK_SECONDS || 300)
  const historyTTL = Number(env.RUN_HISTORY_TTL_SECONDS || 7 * 24 * 3600)

  return async function handle(req) {
    // The scheduler sends no custom headers, so the secret rides in the path: /cron/<secret>/<job>.
    const match = /^\/cron\/([^/]*)\/([A-Za-z0-9_-]+)$/.exec(req.path)
    if (req.method === 'POST' && match) {
      if (!secret || !safeEqual(decodeURIComponent(match[1]), secret)) return { status: 401, body: { error: 'wrong cron secret' } }
      const name = match[2]
      const job = registry[name]
      if (!job) return { status: 404, body: { error: `no job named ${name}` } }

      if ((await kv.get(`lock:${name}`)) !== null) return { status: 409, body: { error: 'previous run still holds the lock' } }
      await kv.put(`lock:${name}`, String(now()), { ttl: lockSeconds })
      const started = now()
      let record
      try {
        const summary = await job({ kv, env, fetch, now })
        record = { job: name, ok: true, summary: summary || '', started_at: new Date(started).toISOString(), duration_ms: now() - started }
      } catch (err) {
        record = { job: name, ok: false, error: String(err.message || err).slice(0, 300), started_at: new Date(started).toISOString(), duration_ms: now() - started }
      } finally {
        await kv.delete(`lock:${name}`)
      }
      await kv.putJSON(`run:${String(started).padStart(15, '0')}:${name}`, record, { ttl: historyTTL })
      return { status: record.ok ? 200 : 500, body: record }
    }

    if (req.method === 'GET' && req.path === '/runs') {
      const keys = (await kv.list('run:', 1000)).sort().reverse().slice(0, 20)
      const runs = []
      for (const key of keys) {
        const run = await kv.getJSON(key)
        if (run) runs.push(run)
      }
      return { status: 200, body: { runs } }
    }

    return { status: 404, body: { error: 'not found' } }
  }
}

module.exports = { createHandler, jobs }
