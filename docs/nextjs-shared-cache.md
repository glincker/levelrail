# Next.js shared cache for multiple replicas

With two or more replicas of a Next.js app, each replica keeps its own ISR page cache and data cache. Replicas then serve different stale content, and `revalidateTag` or `revalidatePath` only reaches the replica that handled the call. The package `@levelrail/next-cache-handler` moves both caches into Redis so every replica reads and invalidates the same entries.

With one replica you do not need it.

## Setup

Install the package and `redis` (node-redis v4 or v5), then add `cache-handler.js` next to `next.config.js`:

```js
module.exports = require('@levelrail/next-cache-handler');
```

```js
// next.config.js
const buildId = process.env.NEXT_BUILD_ID || String(Date.now());
process.env.NEXT_BUILD_ID = buildId;
module.exports = {
  generateBuildId: async () => buildId,
  cacheHandler: require.resolve('./cache-handler.js'),
  cacheMaxMemorySize: 0, // disable the per replica in-memory cache
};
```

The build id is part of every Redis key, so a new deploy never reads pages built by an old build. Set `NEXT_BUILD_ID` in the build step (for example to the git commit) and keep the same value in the runtime environment. If the value differs between build and runtime, the handler reads an empty namespace (safe, but the cache starts cold). Without the variable the handler uses a fixed `nobuild` namespace and old entries can survive across deploys.

## Provision Redis

```
levelrail-cli databases create --name myapp-cache --engine redis --version 7
levelrail-cli databases link myapp-cache myapp --env-var REDIS_URL
```

The handler reads the connection string from the first of `LEVELRAIL_NEXT_CACHE_URL`, `LEVELRAIL_REDIS_URL`, `REDIS_URL`. Run `levelrail-cli databases create --help` for the engine and version values your server offers.

## Settings (environment)

| Variable | Default | Meaning |
| --- | --- | --- |
| `LEVELRAIL_NEXT_CACHE_PREFIX` | `next` | Key namespace per app. Set it when apps share one Redis. |
| `NEXT_BUILD_ID` | `nobuild` | Build id folded into every key. |
| `LEVELRAIL_NEXT_CACHE_TIMEOUT_MS` | 500 | Connect and command timeout. |
| `LEVELRAIL_NEXT_CACHE_MAX_VALUE_BYTES` | 5242880 | Larger entries are not cached. |
| `LEVELRAIL_NEXT_CACHE_LRU_ENTRIES` | 500 | Size of the in-process fallback. |
| `LEVELRAIL_NEXT_CACHE_GRACE_SECONDS` | 86400 | Added to an entry's `revalidate` to form its Redis TTL, so stale-while-revalidate still has something to serve. |
| `LEVELRAIL_NEXT_CACHE_DEFAULT_TTL_SECONDS` | 2592000 | TTL for entries without a revalidate value. |
| `LEVELRAIL_NEXT_CACHE_RETRY_MS` | 5000 | How long to skip Redis after a failure. |

## How it behaves

- Entries are stored as `<prefix>:<buildId>:e:<key>`. Tags are not scanned: each entry records its tags, and one Redis hash holds the last revalidation time per tag. On read the handler compares the entry's `lastModified` with those times and treats an older entry as a miss.
- Tag times use each replica's clock. Keep replica clocks in sync (NTP); a few milliseconds of skew is harmless, seconds can delay or skip an invalidation.
- Fail open: any Redis error, timeout or missing URL switches that replica to a small in-process LRU, logs one line per minute to stderr, and retries Redis after `LEVELRAIL_NEXT_CACHE_RETRY_MS`. Requests are never failed by the cache. While degraded, replicas do not share content, so you get the per replica behaviour until Redis is back; when it returns the fallback is cleared.
- The tag hash has no TTL (dropping it would lose invalidations) and grows by one small field per distinct tag.

## Next.js versions

The handler implements the `get`, `set`, `revalidateTag` and `resetRequestCache` shape used by Next 14 and 15, and also exposes `updateTags` for the Next 16 name. Differences handled: Next 14 passes tags in the entry or in the `x-next-cache-tags` header, Next 15 passes `ctx.tags`; extra `revalidateTag` arguments (Next 16 profiles) are ignored. Next 16 Cache Components (`'use cache'`) use a different interface (`cacheHandlers`) and are not covered by this package.

## Replica test recipe

1. Deploy with 2 replicas and a route that uses `fetch(url, { next: { tags: ['demo'] } })` and prints a timestamp.
2. Request the page repeatedly through the public URL; the timestamp must stay identical on every replica (compare with a response header that shows the container, or hit each replica directly).
3. Call a route that runs `revalidateTag('demo')`. Request again several times: every replica must show the new timestamp once.
4. Stop Redis. Pages keep rendering; each replica may now differ and one warning per minute appears in the logs. Start Redis again and the replicas converge.

## Previews

A pull request preview of a `next.cache: redis` app gets `LEVELRAIL_NEXT_CACHE_PREFIX` set to its own service name (for example `web-pr-12`), so it never reads or writes production keys. A value you set yourself wins. Preview keys are not deleted when the preview is removed: they expire by TTL, or you can flush them by prefix with `redis-cli --scan --pattern 'web-pr-12:*'`.

## Limits

- Node runtime only, not the edge runtime.
- One Redis in one region; no cross region replication or multi region invalidation.
- Not verified against a real multi replica Next.js run. The unit tests use an in-memory fake of the Redis client; the interface shape follows the documented Next 14 and 15 `cacheHandler` contract and has not been exercised by a real `next start` yet. Treat the first deployment as the test.
