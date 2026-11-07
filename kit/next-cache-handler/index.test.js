'use strict';

const test = require('node:test');
const assert = require('node:assert');
const CacheHandler = require('./index.js');

// Minimal stand in for the node-redis calls the handler uses.
function fakeRedis(clock) {
  const kv = new Map();
  const hashes = new Map();
  const f = {
    broken: false,
    kv,
    check() {
      if (f.broken) throw new Error('ECONNREFUSED');
    },
    async get(k) {
      f.check();
      const e = kv.get(k);
      if (!e) return null;
      if (e.exp && e.exp <= clock.t) {
        kv.delete(k);
        return null;
      }
      return e.v;
    },
    async set(k, v, o) {
      f.check();
      kv.set(k, { v, exp: o && o.EX ? clock.t + o.EX * 1000 : 0, ex: o && o.EX });
    },
    async del(k) {
      f.check();
      kv.delete(k);
    },
    async hSet(k, fields) {
      f.check();
      const h = hashes.get(k) || {};
      Object.assign(h, fields);
      hashes.set(k, h);
    },
    async hmGet(k, fields) {
      f.check();
      const h = hashes.get(k) || {};
      return fields.map((x) => (x in h ? h[x] : null));
    },
  };
  return f;
}

function make(extra) {
  const clock = { t: 1000000 };
  const client = fakeRedis(clock);
  const logs = [];
  const h = new CacheHandler({
    client,
    buildId: 'b1',
    keyPrefix: 'app',
    now: () => clock.t,
    logger: (m) => logs.push(m),
    retryMs: 0,
    ...extra,
  });
  return { clock, client, logs, h };
}

test('set and get roundtrip keeps buffers', async () => {
  const { h } = make();
  const data = { kind: 'APP_PAGE', html: '<p>x</p>', body: Buffer.from('hello'), headers: {} };
  await h.set('/a', data, { revalidate: 60 });
  const got = await h.get('/a');
  assert.ok(Buffer.isBuffer(got.value.body));
  assert.strictEqual(got.value.body.toString(), 'hello');
  assert.strictEqual(got.value.html, '<p>x</p>');
  assert.strictEqual(await h.get('/missing'), null);
});

test('ttl is revalidate plus grace and entries expire', async () => {
  const { h, clock, client } = make({ graceSeconds: 10 });
  await h.set('/a', { kind: 'FETCH', data: 1 }, { revalidate: 5 });
  const stored = [...client.kv.values()][0];
  assert.strictEqual(stored.ex, 15);
  clock.t += 14000;
  assert.ok(await h.get('/a'));
  clock.t += 2000;
  assert.strictEqual(await h.get('/a'), null);
});

test('tag revalidation invalidates entries set before the tag time', async () => {
  const { h, clock } = make();
  await h.set('/a', { kind: 'FETCH', data: 1 }, { tags: ['posts'] });
  await h.set('/b', { kind: 'APP_PAGE', headers: { 'x-next-cache-tags': 'posts,other' } }, {});
  await h.set('/c', { kind: 'FETCH', data: 3 }, { tags: ['unrelated'] });
  clock.t += 10;
  await h.revalidateTag('posts');
  assert.strictEqual(await h.get('/a'), null);
  assert.strictEqual(await h.get('/b'), null);
  assert.ok(await h.get('/c'));
  clock.t += 10;
  await h.set('/a', { kind: 'FETCH', data: 2 }, { tags: ['posts'] });
  assert.strictEqual((await h.get('/a')).value.data, 2);
});

test('soft tags from the request invalidate too', async () => {
  const { h, clock } = make();
  await h.set('/a', { kind: 'FETCH' }, {});
  clock.t += 5;
  await h.revalidateTag(['_N_T_/a']);
  assert.strictEqual(await h.get('/a', { softTags: ['_N_T_/a'] }), null);
});

test('replicas sharing one client see each other writes and revalidation', async () => {
  const { h, clock, client } = make();
  const h2 = new CacheHandler({ client, buildId: 'b1', keyPrefix: 'app', now: () => clock.t });
  await h.set('/a', { kind: 'FETCH' }, { tags: ['t'] });
  assert.ok(await h2.get('/a'));
  clock.t += 1;
  await h2.revalidateTag('t');
  assert.strictEqual(await h.get('/a'), null);
});

test('redis failure falls back to the lru and logs once per minute', async () => {
  const { h, client, logs, clock } = make();
  client.broken = true;
  await h.set('/a', { kind: 'FETCH', data: 1 }, { tags: ['t'] });
  assert.strictEqual((await h.get('/a')).value.data, 1);
  clock.t += 1;
  await h.revalidateTag('t');
  assert.strictEqual(await h.get('/a'), null);
  await h.get('/x');
  await h.get('/y');
  assert.strictEqual(logs.length, 1);
  clock.t += 61000;
  await h.get('/z');
  assert.strictEqual(logs.length, 2);
});

test('recovery clears the fallback lru', async () => {
  const { h, client } = make();
  client.broken = true;
  await h.set('/a', { kind: 'FETCH' }, {});
  client.broken = false;
  assert.strictEqual(await h.get('/a'), null);
  assert.strictEqual(h.lru.size, 0);
});

test('lru is bounded', async () => {
  const { h, client } = make({ lruEntries: 2 });
  client.broken = true;
  for (const k of ['/1', '/2', '/3']) await h.set(k, { kind: 'FETCH' }, {});
  assert.strictEqual(await h.get('/1'), null);
  assert.ok(await h.get('/3'));
});

test('oversized values are not cached', async () => {
  const { h, client } = make({ maxValueBytes: 50 });
  await h.set('/big', { kind: 'FETCH', data: 'x'.repeat(200) }, {});
  assert.strictEqual(client.kv.size, 0);
  assert.strictEqual(await h.get('/big'), null);
});

test('prefix and build id isolate entries', async () => {
  const { h, clock, client } = make();
  await h.set('/a', { kind: 'FETCH' }, {});
  const other = new CacheHandler({ client, buildId: 'b2', keyPrefix: 'app', now: () => clock.t });
  const otherApp = new CacheHandler({ client, buildId: 'b1', keyPrefix: 'app2', now: () => clock.t });
  assert.strictEqual(await other.get('/a'), null);
  assert.strictEqual(await otherApp.get('/a'), null);
  assert.ok(await h.get('/a'));
});

test('set with null deletes the entry', async () => {
  const { h } = make();
  await h.set('/a', { kind: 'FETCH' }, {});
  await h.set('/a', null, {});
  assert.strictEqual(await h.get('/a'), null);
});

test('instances in one process share the fallback and the log throttle', async () => {
  const { client, logs, clock } = make({ stateKey: 'shared-1' });
  const mk = () => new CacheHandler({ client, stateKey: 'shared-1', buildId: 'b1', keyPrefix: 'app', now: () => clock.t, logger: (m) => logs.push(m), retryMs: 0 });
  client.broken = true;
  const a = mk();
  const b = mk();
  await a.set('/p', { kind: 'APP_PAGE', html: 'x', headers: {} }, { revalidate: 60 });
  assert.strictEqual((await b.get('/p')).value.html, 'x');
  for (let i = 0; i < 6; i++) await mk().get('/q' + i);
  assert.strictEqual(logs.length, 1);
});

test('revalidation made during an outage is applied to redis on reconnect', async () => {
  const { h, client, clock } = make();
  await h.set('/a', { kind: 'FETCH', data: 1 }, { tags: ['t'] });
  clock.t += 10;
  client.broken = true;
  await h.revalidateTag('t');
  assert.strictEqual(await h.get('/a'), null);
  client.broken = false;
  clock.t += 10;
  const other = new CacheHandler({ client, buildId: 'b1', keyPrefix: 'app', now: () => clock.t });
  assert.strictEqual(await h.get('/a'), null);
  assert.strictEqual(await other.get('/a'), null);
  assert.strictEqual(h.localTags.size, 0);
});

test('fallback reads honor request soft tags revalidated during the outage', async () => {
  const { h, client, clock } = make();
  client.broken = true;
  await h.set('/a', { kind: 'FETCH', data: 1 }, {});
  clock.t += 5;
  await h.revalidateTag(['_N_T_/a']);
  assert.strictEqual(await h.get('/a', { softTags: ['_N_T_/a'] }), null);
  assert.ok(await h.get('/a', { softTags: [] }));
});

test('a revalidation stored while recovery is in flight still reaches redis', async () => {
  const { h, client, clock } = make();
  client.broken = true;
  await h.revalidateTag('old');
  client.broken = false;
  clock.t += 5;
  let release;
  const gate = new Promise((r) => {
    release = r;
  });
  let reached;
  const inGet = new Promise((r) => {
    reached = r;
  });
  const realGet = client.get.bind(client);
  client.get = async (k) => {
    reached();
    await gate;
    return realGet(k);
  };
  const first = h.get('/x');
  await inGet;
  const realHSet = client.hSet.bind(client);
  client.hSet = async () => {
    throw new Error('ECONNRESET');
  };
  clock.t += 5;
  await h.revalidateTag('fresh');
  client.hSet = realHSet;
  client.get = realGet;
  release();
  await first;
  assert.ok(h.localTags.has('fresh'));
  assert.strictEqual(h.degraded, true);
  await h.get('/y');
  const stored = await client.hmGet('app:b1:tags', ['old', 'fresh']);
  assert.ok(stored[0] && stored[1]);
  assert.strictEqual(h.localTags.size, 0);
  assert.strictEqual(h.degraded, false);
});

test('replay keeps a newer tag time already stored', async () => {
  const { h, client, clock } = make();
  client.broken = true;
  await h.revalidateTag('t');
  client.broken = false;
  clock.t += 50;
  const other = new CacheHandler({ client, buildId: 'b1', keyPrefix: 'app', now: () => clock.t });
  await other.revalidateTag('t');
  const newer = String(clock.t);
  await h.get('/x');
  const stored = await client.hmGet('app:b1:tags', ['t']);
  assert.strictEqual(stored[0], newer);
});

test('the database CA comes from an option, PEM env or file and verification stays on', () => {
  const pem = '-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n';
  const { h } = make({ ca: pem });
  assert.strictEqual(h.ca, pem);
  process.env.LEVELRAIL_NEXT_CACHE_CA = pem;
  try {
    assert.strictEqual(make().h.ca, pem);
  } finally {
    delete process.env.LEVELRAIL_NEXT_CACHE_CA;
  }
  assert.strictEqual(make().h.ca, undefined);
  process.env.LEVELRAIL_NEXT_CACHE_CA_FILE = '/nonexistent/ca.pem';
  try {
    const m = make();
    assert.strictEqual(m.h.ca, undefined);
    assert.strictEqual(m.logs.length, 1);
  } finally {
    delete process.env.LEVELRAIL_NEXT_CACHE_CA_FILE;
  }
});

// Cache Components (cacheHandlers) contract.
const { ComponentsCacheHandler, componentsHandler } = CacheHandler;

function streamOf(text) {
  return new ReadableStream({
    start(c) {
      c.enqueue(new TextEncoder().encode(text));
      c.close();
    },
  });
}

async function textOf(stream) {
  const reader = stream.getReader();
  let out = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    out += Buffer.from(value).toString();
  }
  return out;
}

function makeCc(extra) {
  const clock = { t: 1000000 };
  const client = fakeRedis(clock);
  const logs = [];
  const h = new ComponentsCacheHandler({ client, buildId: 'b1', keyPrefix: 'app', now: () => clock.t, logger: (m) => logs.push(m), retryMs: 0, ...extra });
  const entry = (text, over) => ({ value: streamOf(text), tags: ['posts'], stale: 30, timestamp: clock.t, expire: 600, revalidate: 60, ...over });
  return { clock, client, logs, h, entry };
}

test('components: set reads the stream and get returns a fresh one', async () => {
  const { h, entry } = makeCc();
  await h.set('k', Promise.resolve(entry('hello')));
  const a = await h.get('k', []);
  assert.strictEqual(await textOf(a.value), 'hello');
  assert.deepStrictEqual(a.tags, ['posts']);
  assert.strictEqual(a.revalidate, 60);
  assert.strictEqual(await textOf((await h.get('k', [])).value), 'hello');
  assert.strictEqual(await h.get('missing', []), undefined);
});

test('components: keys share the prefix and build id, apart from classic keys', async () => {
  const { h, client, entry } = makeCc();
  await h.set('k', Promise.resolve(entry('x')));
  assert.deepStrictEqual([...client.kv.keys()], ['app:b1:c:k']);
  assert.strictEqual(client.kv.get('app:b1:c:k').ex, 600);
});

test('components: updateTags invalidates and getExpiration reports the newest time', async () => {
  const { h, clock, entry } = makeCc();
  await h.set('k', Promise.resolve(entry('x')));
  assert.strictEqual(await h.getExpiration(['posts']), 0);
  clock.t += 10;
  await h.updateTags(['posts'], {});
  assert.strictEqual(await h.get('k', []), undefined);
  assert.strictEqual(await h.getExpiration(['posts', 'other']), clock.t);
  await h.refreshTags();
});

test('components: soft tags and classic revalidateTag both invalidate', async () => {
  const { h, clock, client, entry } = makeCc();
  await h.set('k', Promise.resolve(entry('x', { tags: [] })));
  clock.t += 5;
  await h.updateTags(['_N_T_/a']);
  assert.strictEqual(await h.get('k', ['_N_T_/a']), undefined);
  const classic = new CacheHandler({ client, buildId: 'b1', keyPrefix: 'app', now: () => clock.t });
  await h.set('j', Promise.resolve(entry('y')));
  clock.t += 5;
  await classic.revalidateTag('posts');
  assert.strictEqual(await h.get('j', []), undefined);
});

test('components: expired entries are misses', async () => {
  const { h, clock, entry } = makeCc();
  await h.set('k', Promise.resolve(entry('x', { expire: 5 })));
  clock.t += 6000;
  assert.strictEqual(await h.get('k', []), undefined);
});

test('components: outage falls back to the LRU and replays tags', async () => {
  const { h, clock, client, entry, logs } = makeCc();
  client.broken = true;
  await h.set('k', Promise.resolve(entry('local')));
  assert.strictEqual(await textOf((await h.get('k', [])).value), 'local');
  assert.ok(logs.length >= 1);
  clock.t += 10;
  await h.updateTags(['posts']);
  assert.strictEqual(await h.get('k', []), undefined);
  assert.strictEqual(await h.getExpiration(['posts']), clock.t);
  client.broken = false;
  clock.t += 10;
  assert.strictEqual(await h.get('k', []), undefined);
  assert.strictEqual(h.degraded, false);
});

test('components: oversize and unreadable entries are skipped, never thrown', async () => {
  const { h, client, entry } = makeCc({ maxValueBytes: 20 });
  await h.set('big', Promise.resolve(entry('x'.repeat(100))));
  assert.strictEqual(client.kv.size, 0);
  await h.set('bad', Promise.reject(new Error('render failed')));
  assert.strictEqual(client.kv.size, 0);
});

test('components: exported plain object exposes the five hooks', () => {
  for (const m of ['get', 'set', 'refreshTags', 'getExpiration', 'updateTags']) {
    assert.strictEqual(typeof componentsHandler[m], 'function');
  }
});
