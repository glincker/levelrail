'use strict';

// Redis backed Next.js cacheHandler. Fails open: any Redis problem degrades to a
// small per process LRU and never throws into the request path.

const fs = require('node:fs');

const DEFAULTS = {
  timeoutMs: 500,
  maxValueBytes: 5 * 1024 * 1024,
  lruEntries: 500,
  graceSeconds: 86400,
  defaultTtlSeconds: 30 * 86400,
  retryMs: 5000,
  logIntervalMs: 60000,
};

function envInt(name, fallback) {
  const raw = process.env[name];
  if (raw === undefined || raw === '') return fallback;
  const n = Number(raw);
  return Number.isFinite(n) && n >= 0 ? n : fallback;
}

function encode(obj) {
  return JSON.stringify(obj, function (k, v) {
    const raw = this[k];
    if (Buffer.isBuffer(raw)) return { __b64: raw.toString('base64') };
    return v;
  });
}

function decode(text) {
  return JSON.parse(text, (k, v) => {
    if (v && typeof v === 'object' && typeof v.__b64 === 'string') return Buffer.from(v.__b64, 'base64');
    return v;
  });
}

// Page and route entries carry their tags in a response header.
function collectTags(data, ctx) {
  const out = new Set();
  const add = (list) => {
    if (Array.isArray(list)) list.forEach((t) => typeof t === 'string' && out.add(t));
  };
  add(ctx && ctx.tags);
  add(data && data.tags);
  const header = data && data.headers && data.headers['x-next-cache-tags'];
  if (typeof header === 'string') add(header.split(','));
  return [...out];
}

// Next builds several handler instances per process, so Redis state, the fallback
// LRU, pending revalidations and the log throttle live here, keyed by namespace.
const shared = new Map();

function newState() {
  return { client: null, connecting: null, downUntil: 0, lastLog: 0, lru: new Map(), localTags: new Map(), degraded: false };
}

function sharedState(key) {
  let st = shared.get(key);
  if (!st) {
    st = newState();
    shared.set(key, st);
  }
  return st;
}

// PEM text wins over a file path; an unreadable file is reported and ignored (verification stays on).
function loadCa(o, log) {
  if (o.ca) return o.ca;
  const pem = process.env.LEVELRAIL_NEXT_CACHE_CA;
  if (pem && pem.includes('BEGIN CERTIFICATE')) return pem;
  const file = process.env.LEVELRAIL_NEXT_CACHE_CA_FILE;
  if (!file) return undefined;
  try {
    return fs.readFileSync(file, 'utf8');
  } catch (err) {
    log(`cannot read LEVELRAIL_NEXT_CACHE_CA_FILE: ${err && err.message}`);
    return undefined;
  }
}

class CacheHandler {
  constructor(options) {
    const o = options && typeof options === 'object' ? options : {};
    this.now = o.now || Date.now;
    this.logger = o.logger || ((m) => process.stderr.write(m + '\n'));
    this.timeoutMs = o.timeoutMs ?? envInt('LEVELRAIL_NEXT_CACHE_TIMEOUT_MS', DEFAULTS.timeoutMs);
    this.maxValueBytes = o.maxValueBytes ?? envInt('LEVELRAIL_NEXT_CACHE_MAX_VALUE_BYTES', DEFAULTS.maxValueBytes);
    this.lruMax = o.lruEntries ?? envInt('LEVELRAIL_NEXT_CACHE_LRU_ENTRIES', DEFAULTS.lruEntries);
    this.graceSeconds = o.graceSeconds ?? envInt('LEVELRAIL_NEXT_CACHE_GRACE_SECONDS', DEFAULTS.graceSeconds);
    this.defaultTtl = o.defaultTtlSeconds ?? envInt('LEVELRAIL_NEXT_CACHE_DEFAULT_TTL_SECONDS', DEFAULTS.defaultTtlSeconds);
    this.retryMs = o.retryMs ?? envInt('LEVELRAIL_NEXT_CACHE_RETRY_MS', DEFAULTS.retryMs);
    const app = o.keyPrefix ?? process.env.LEVELRAIL_NEXT_CACHE_PREFIX ?? 'next';
    const build = o.buildId ?? process.env.NEXT_BUILD_ID ?? 'nobuild';
    this.prefix = `${app}:${build}:`;
    this.tagsKey = `${this.prefix}tags`;
    this.url = o.redisUrl || process.env.LEVELRAIL_NEXT_CACHE_URL || process.env.LEVELRAIL_REDIS_URL || process.env.REDIS_URL || '';
    this.servername = o.servername || process.env.LEVELRAIL_NEXT_CACHE_CA_SERVERNAME || '';
    // An injected client without a stateKey keeps private state (unit tests, embedding).
    this.st = o.client && !o.stateKey ? newState() : sharedState(o.stateKey || `${this.prefix}|${this.url}`);
    if (o.client) this.st.client = o.client;
    this.ca = loadCa(o, (m) => this.log(m));
  }

  get lru() {
    return this.st.lru;
  }

  get localTags() {
    return this.st.localTags;
  }

  get degraded() {
    return this.st.degraded;
  }

  log(msg) {
    const t = this.now();
    if (this.st.lastLog && t - this.st.lastLog < DEFAULTS.logIntervalMs) return;
    this.st.lastLog = t;
    try {
      this.logger(`levelrail next-cache: ${msg}`);
    } catch {
      // logging must never break a request
    }
  }

  withTimeout(promise) {
    let timer;
    const limit = new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error('redis command timeout')), this.timeoutMs);
    });
    return Promise.race([promise, limit]).finally(() => clearTimeout(timer));
  }

  async getClient() {
    const st = this.st;
    if (st.client) {
      if (!st.connecting && typeof st.client.connect === 'function' && st.client.isOpen === false) {
        st.connecting = Promise.resolve(st.client.connect());
      }
      if (st.connecting) await this.withTimeout(st.connecting);
      return st.client;
    }
    if (!this.url) throw new Error('no redis url (set LEVELRAIL_REDIS_URL or REDIS_URL)');
    if (!st.connecting) {
      // Loaded lazily so the peer dependency is only needed when Redis is used.
      const { createClient } = require('redis');
      const socket = { connectTimeout: this.timeoutMs, reconnectStrategy: (n) => Math.min(n * 100, 3000) };
      // Trust the database CA for this connection only; verification is never switched off.
      if (this.ca) socket.ca = this.ca;
      if (this.ca && this.servername) socket.servername = this.servername;
      const client = createClient({ url: this.url, socket });
      client.on('error', (err) => this.log(`redis error: ${err && err.message}`));
      st.connecting = Promise.resolve(client.connect()).then(() => {
        st.client = client;
      });
      st.connecting.catch(() => {
        st.connecting = null;
      });
    }
    await this.withTimeout(st.connecting);
    return st.client;
  }

  // Applies revalidations made while Redis was away, keeping any newer time already stored.
  async replayTags(c) {
    if (!this.st.localTags.size) return;
    const tags = [...this.st.localTags.keys()];
    const stored = await c.hmGet(this.tagsKey, tags);
    const fields = {};
    tags.forEach((tag, i) => {
      const have = Number(stored[i]);
      const mine = this.st.localTags.get(tag);
      fields[tag] = String(Number.isFinite(have) && have > mine ? have : mine);
    });
    await c.hSet(this.tagsKey, fields);
  }

  // Returns {ok, value}. ok=false means Redis was unavailable and the caller uses the LRU.
  async run(fn) {
    if (this.now() < this.st.downUntil) return { ok: false };
    try {
      const client = await this.getClient();
      // Replay before the command so the first read after an outage already sees the revalidation.
      if (this.st.degraded) await this.withTimeout(Promise.resolve(this.replayTags(client)));
      const value = await this.withTimeout(Promise.resolve(fn(client)));
      if (this.st.degraded) {
        this.st.degraded = false;
        this.st.lru.clear();
        this.st.localTags.clear();
      }
      return { ok: true, value };
    } catch (err) {
      this.st.degraded = true;
      this.st.downUntil = this.now() + this.retryMs;
      this.log(`redis unavailable, using in-process fallback: ${err && err.message}`);
      return { ok: false };
    }
  }

  lruGet(key) {
    const e = this.lru.get(key);
    if (!e) return null;
    this.lru.delete(key);
    if (e.expiresAt <= this.now()) return null;
    this.lru.set(key, e);
    return e;
  }

  lruSet(key, entry) {
    this.lru.delete(key);
    this.lru.set(key, entry);
    while (this.lru.size > this.lruMax) this.lru.delete(this.lru.keys().next().value);
  }

  staleByTag(tags, lastModified, times) {
    return tags.some((t) => {
      const ts = Number(times[t]);
      return Number.isFinite(ts) && ts > lastModified;
    });
  }

  async get(key, ctx) {
    const rkey = this.prefix + 'e:' + key;
    const soft = (ctx && Array.isArray(ctx.softTags) ? ctx.softTags : []).concat(ctx && Array.isArray(ctx.tags) ? ctx.tags : []);
    const res = await this.run(async (c) => {
      const raw = await c.get(rkey);
      if (!raw) return null;
      const entry = decode(raw);
      const tags = [...new Set([...(entry.tags || []), ...soft])];
      if (tags.length) {
        const stamps = await c.hmGet(this.tagsKey, tags);
        const times = {};
        tags.forEach((t, i) => {
          times[t] = stamps[i];
        });
        if (this.staleByTag(tags, entry.lastModified, times)) return null;
      }
      return entry;
    });
    if (res.ok) return res.value ? { value: res.value.value, lastModified: res.value.lastModified } : null;
    const e = this.lruGet(key);
    if (!e) return null;
    const times = Object.fromEntries(this.localTags);
    if (this.staleByTag(e.tags, e.lastModified, times)) return null;
    return { value: e.value, lastModified: e.lastModified };
  }

  async set(key, data, ctx) {
    const rkey = this.prefix + 'e:' + key;
    if (data === null || data === undefined) {
      this.lru.delete(key);
      await this.run((c) => c.del(rkey));
      return;
    }
    const tags = collectTags(data, ctx);
    const lastModified = this.now();
    const reval = ctx && typeof ctx.revalidate === 'number' && ctx.revalidate > 0 ? ctx.revalidate : 0;
    const ttl = reval ? Math.ceil(reval) + this.graceSeconds : this.defaultTtl;
    let payload;
    try {
      payload = encode({ value: data, lastModified, tags });
    } catch (err) {
      this.log(`entry not serializable, skipped: ${err && err.message}`);
      return;
    }
    if (Buffer.byteLength(payload) > this.maxValueBytes) {
      this.log('entry over LEVELRAIL_NEXT_CACHE_MAX_VALUE_BYTES, not cached');
      return;
    }
    const res = await this.run((c) => (ttl > 0 ? c.set(rkey, payload, { EX: ttl }) : c.set(rkey, payload)));
    if (!res.ok) {
      this.lruSet(key, { value: data, lastModified, tags, expiresAt: lastModified + (ttl || this.defaultTtl) * 1000 });
    }
  }

  async revalidateTag(tags) {
    const list = (Array.isArray(tags) ? tags : [tags]).filter((t) => typeof t === 'string' && t);
    if (!list.length) return;
    const t = this.now();
    const fields = {};
    list.forEach((tag) => {
      fields[tag] = String(t);
    });
    const res = await this.run((c) => c.hSet(this.tagsKey, fields));
    if (!res.ok) list.forEach((tag) => this.localTags.set(tag, t));
  }

  // Next 16 renamed the hook; both names do the same thing here.
  async updateTags(tags) {
    return this.revalidateTag(tags);
  }

  resetRequestCache() {}
}

// Reads a stream of byte chunks into one Buffer.
async function drain(stream) {
  const reader = stream.getReader();
  const parts = [];
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    parts.push(Buffer.from(value));
  }
  return Buffer.concat(parts);
}

function toStream(buf) {
  return new ReadableStream({
    start(controller) {
      controller.enqueue(new Uint8Array(buf));
      controller.close();
    },
  });
}

// Next 16 Cache Components handler ('use cache'). Shares the Redis client, fallback LRU,
// key prefix and tag times with CacheHandler; entries live under their own "c:" keys.
class ComponentsCacheHandler extends CacheHandler {
  ckey(key) {
    return this.prefix + 'c:' + key;
  }

  async get(cacheKey, softTags) {
    const key = this.ckey(cacheKey);
    const soft = Array.isArray(softTags) ? softTags : [];
    const res = await this.run(async (c) => {
      const raw = await c.get(key);
      if (!raw) return null;
      const entry = decode(raw);
      const tags = [...new Set([...(entry.tags || []), ...soft])];
      if (tags.length) {
        const stamps = await c.hmGet(this.tagsKey, tags);
        const times = {};
        tags.forEach((t, i) => {
          times[t] = stamps[i];
        });
        if (this.staleByTag(tags, entry.timestamp, times)) return null;
      }
      return entry;
    });
    let entry = res.ok ? res.value : null;
    if (!res.ok) {
      const e = this.lruGet(key);
      if (!e) return undefined;
      const tags = [...new Set([...(e.tags || []), ...soft])];
      if (this.staleByTag(tags, e.timestamp, Object.fromEntries(this.localTags))) return undefined;
      entry = e;
    }
    if (!entry) return undefined;
    if (entry.expire > 0 && this.now() > entry.timestamp + entry.expire * 1000) return undefined;
    return {
      value: toStream(Buffer.from(entry.value)),
      tags: entry.tags,
      stale: entry.stale,
      timestamp: entry.timestamp,
      expire: entry.expire,
      revalidate: entry.revalidate,
    };
  }

  async set(cacheKey, pendingEntry) {
    const key = this.ckey(cacheKey);
    let entry;
    let body;
    try {
      entry = await pendingEntry;
      body = await drain(entry.value);
    } catch (err) {
      this.log(`cache component entry not readable, skipped: ${err && err.message}`);
      return;
    }
    const tags = Array.isArray(entry.tags) ? entry.tags.filter((t) => typeof t === 'string') : [];
    const stored = {
      value: body,
      tags,
      stale: entry.stale,
      timestamp: entry.timestamp,
      expire: entry.expire,
      revalidate: entry.revalidate,
    };
    const payload = encode({ ...stored, value: { __b64: body.toString('base64') } });
    if (Buffer.byteLength(payload) > this.maxValueBytes) {
      this.log('entry over LEVELRAIL_NEXT_CACHE_MAX_VALUE_BYTES, not cached');
      return;
    }
    const ttl = entry.expire > 0 ? Math.ceil(entry.expire) : this.defaultTtl;
    const res = await this.run((c) => (ttl > 0 ? c.set(key, payload, { EX: ttl }) : c.set(key, payload)));
    if (!res.ok) this.lruSet(key, { ...stored, lastModified: entry.timestamp, expiresAt: this.now() + (ttl || this.defaultTtl) * 1000 });
  }

  // Reads happen in get, so there is no periodic sync to do.
  async refreshTags() {}

  async getExpiration(tags) {
    const list = (Array.isArray(tags) ? tags : []).filter((t) => typeof t === 'string' && t);
    if (!list.length) return 0;
    const res = await this.run((c) => c.hmGet(this.tagsKey, list));
    const stamps = res.ok ? res.value : list.map((t) => this.localTags.get(t));
    return stamps.reduce((max, v) => {
      const n = Number(v);
      return Number.isFinite(n) && n > max ? n : max;
    }, 0);
  }

  async updateTags(tags) {
    return this.revalidateTag(tags);
  }
}

// Next loads cacheHandlers entries as plain objects, so build one lazily (env is read on first use).
function createComponentsHandler(options) {
  let inner;
  const h = () => inner || (inner = new ComponentsCacheHandler(options));
  return {
    get: (key, softTags) => h().get(key, softTags),
    set: (key, pending) => h().set(key, pending),
    refreshTags: () => h().refreshTags(),
    getExpiration: (tags) => h().getExpiration(tags),
    updateTags: (tags, durations) => h().updateTags(tags, durations),
  };
}

module.exports = CacheHandler;
module.exports.default = CacheHandler;
module.exports.CacheHandler = CacheHandler;
module.exports.ComponentsCacheHandler = ComponentsCacheHandler;
module.exports.createComponentsHandler = createComponentsHandler;
module.exports.componentsHandler = createComponentsHandler();
