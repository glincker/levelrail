import assert from "node:assert/strict";
import { test } from "node:test";
import { Levelrail, LevelrailApiError, collect } from "../src/index.ts";
import { fakeServer, json } from "./helpers.ts";

const fast = { retryBaseMs: 1, retryMaxMs: 50 };

test("sends bearer token, path params and query", async () => {
  const s = await fakeServer((_r, res) => json(res, 200, { ok: true }));
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "tok", ...fast });
    const out = await lr.deploys.getDeploy({ path: { name: "my app", deployId: "d1" }, query: { verbose: true } });
    assert.deepEqual(out, { ok: true });
    assert.equal(s.seen[0]?.headers.authorization, "Bearer tok");
    assert.equal(s.seen[0]?.url, "/api/v1/apps/my%20app/deploys/d1?verbose=true");
  } finally {
    await s.close();
  }
});

test("posts a JSON body with an idempotency key", async () => {
  const s = await fakeServer((_r, res) => json(res, 201, { name: "web" }));
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", ...fast });
    await lr.apps.createApp({ body: { name: "web", image: "nginx" } }, { idempotencyKey: "k1" });
    assert.equal(s.seen[0]?.headers["idempotency-key"], "k1");
    assert.equal(s.seen[0]?.headers["content-type"], "application/json");
    assert.deepEqual(JSON.parse(s.seen[0]?.body ?? ""), { name: "web", image: "nginx" });
  } finally {
    await s.close();
  }
});

test("retries 429 honoring Retry-After and 503 on GET", async () => {
  const s = await fakeServer((_r, res, n) => {
    if (n === 1) return json(res, 429, { code: "rate_limited", error: "slow down" }, { "retry-after": "0" });
    if (n === 2) return json(res, 503, { error: "busy" });
    json(res, 200, [{ name: "a" }]);
  });
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", maxRetries: 3, ...fast });
    assert.deepEqual(await lr.apps.listApps(), [{ name: "a" }]);
    assert.equal(s.seen.length, 3);
  } finally {
    await s.close();
  }
});

test("does not retry a 5xx POST without an idempotency key", async () => {
  const s = await fakeServer((_r, res) => json(res, 502, { error: "bad gateway" }));
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", maxRetries: 2, ...fast });
    await assert.rejects(lr.apps.createApp({ body: { name: "x" } }), (e: LevelrailApiError) => e.status === 502);
    assert.equal(s.seen.length, 1);
    await assert.rejects(lr.apps.createApp({ body: { name: "x" } }, { idempotencyKey: "k" }));
    assert.equal(s.seen.length, 4);
  } finally {
    await s.close();
  }
});

test("maps API errors to typed errors", async () => {
  const s = await fakeServer((_r, res) => json(res, 404, { code: "not_found", error: "app not found", request_id: "r-1", fix: "create it first" }));
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", ...fast });
    await assert.rejects(lr.apps.listApps(), (e: unknown) => {
      assert.ok(e instanceof LevelrailApiError);
      assert.equal(e.status, 404);
      assert.equal(e.code, "not_found");
      assert.equal(e.message, "app not found");
      assert.equal(e.requestId, "r-1");
      assert.equal(e.fix, "create it first");
      return true;
    });
  } finally {
    await s.close();
  }
});

test("gives up after maxRetries on 429 and exposes retryAfterMs", async () => {
  const s = await fakeServer((_r, res) => json(res, 429, { error: "no" }, { "retry-after": "0" }));
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", maxRetries: 1, ...fast });
    await assert.rejects(lr.apps.listApps(), (e: LevelrailApiError) => e.isRateLimited && e.retryAfterMs === 0 && e.code === "rate_limited");
    assert.equal(s.seen.length, 2);
  } finally {
    await s.close();
  }
});

test("paginates limit/offset until a short page", async () => {
  const all = Array.from({ length: 5 }, (_, i) => ({ id: i }));
  const s = await fakeServer((r, res) => {
    const u = new URL(r.url, "http://x");
    const limit = Number(u.searchParams.get("limit"));
    const offset = Number(u.searchParams.get("offset"));
    json(res, 200, { items: all.slice(offset, offset + limit) });
  });
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", ...fast });
    const got = await collect(lr.paginate("handleListApps", undefined, { pageSize: 2 }));
    assert.deepEqual(got, all);
    assert.equal(s.seen.length, 3);
    assert.equal((await lr.listAll("handleListApps", undefined, { pageSize: 2, maxItems: 3 })).length, 3);
  } finally {
    await s.close();
  }
});

test("abort cancels an in-flight request", async () => {
  const s = await fakeServer(() => undefined);
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", ...fast });
    const ctl = new AbortController();
    const p = lr.apps.listApps(undefined, { signal: ctl.signal });
    setTimeout(() => ctl.abort(), 20);
    await assert.rejects(p, (e: Error) => e.name === "AbortError");
  } finally {
    await s.close();
  }
});

test("abort during a retry wait stops retrying", async () => {
  const s = await fakeServer((_r, res) => json(res, 429, { error: "wait" }, { "retry-after": "5" }));
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", retryBaseMs: 1, retryMaxMs: 10_000 });
    const ctl = new AbortController();
    const p = lr.apps.listApps(undefined, { signal: ctl.signal });
    setTimeout(() => ctl.abort(), 30);
    await assert.rejects(p, (e: Error) => e.name === "AbortError");
    assert.equal(s.seen.length, 1);
  } finally {
    await s.close();
  }
});

test("streams server-sent events and parses JSON payloads", async () => {
  const s = await fakeServer((_r, res) => {
    res.writeHead(200, { "content-type": "text/event-stream" });
    res.write('event: status\ndata: {"state":"building"}\n\n');
    res.write("data: plain text\n\n");
    res.end('id: 3\nevent: done\ndata: {"state":"ready"}\n\n');
  });
  try {
    const lr = new Levelrail({ baseUrl: s.baseUrl, token: "t", ...fast });
    const events = await collect(lr.deploys.deployLogStream({ path: { name: "web", deployId: "d1" } }));
    assert.deepEqual(events.map((e) => e.event), ["status", "message", "done"]);
    assert.deepEqual(events[0]?.data, { state: "building" });
    assert.equal(events[1]?.data, "plain text");
    assert.equal(events[2]?.id, "3");
    assert.equal(s.seen[0]?.headers.accept, "text/event-stream");
  } finally {
    await s.close();
  }
});

test("requires a base URL and every path parameter", () => {
  assert.throws(() => new Levelrail({ baseUrl: "" }), /baseUrl/);
  const lr = new Levelrail({ baseUrl: "http://127.0.0.1:1", token: "t" });
  assert.throws(() => (lr.call as (id: string, a: object) => unknown)("handleGetDeploy", { path: { name: "x" } }), /deployId/);
});
