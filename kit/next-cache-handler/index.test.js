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
