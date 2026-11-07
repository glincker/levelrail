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

The handler reads the connection string from the first of `LEVELRAIL_NEXT_CACHE_URL`, `LEVELRAIL_REDIS_URL`, `REDIS_URL`.

### TLS

A managed Redis serves TLS only, with a self-signed certificate, so the link URL is `rediss://`. The handler verifies that certificate; it never turns verification off. With `next.cache: redis`, Levelrail links the database certificate to `LEVELRAIL_NEXT_CACHE_CA` (database field `tls_ca`) for you, next to `LEVELRAIL_NEXT_CACHE_URL`, and the handler trusts it for this connection only. No manual step is needed.

The CA link is added only when the linked Redis has TLS on and supports the field, and it is removed when `next.cache` is set to `none`. If you set `LEVELRAIL_NEXT_CACHE_CA` yourself, your value wins. The certificate is issued for the database container name, so use the same-node host the link provides. For a host with a different name, set `LEVELRAIL_NEXT_CACHE_CA_SERVERNAME` to the certificate name. Restart the app after changing the database TLS setting. To link the field by hand (for example with a custom variable name), run `levelrail-cli databases link myapp-cache myapp --env-var LEVELRAIL_NEXT_CACHE_CA --field tls_ca`; in `app.yaml` the same reference is `LEVELRAIL_NEXT_CACHE_CA: { from: { database: myapp-cache, field: tls_ca } }`.

Without the CA the handler logs a certificate error and falls back to per process caching. Do not disable TLS on the database to work around it. Run `levelrail-cli databases create --help` for the engine and version values your server offers.

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
| `LEVELRAIL_NEXT_CACHE_CA` | unset | PEM of the CA to trust for a `rediss://` URL (see TLS). |
| `LEVELRAIL_NEXT_CACHE_CA_FILE` | unset | Path to a PEM file, used when `LEVELRAIL_NEXT_CACHE_CA` is unset. |
| `LEVELRAIL_NEXT_CACHE_CA_SERVERNAME` | unset | Certificate name to verify when the host differs from it. |

## How it behaves

- Entries are stored as `<prefix>:<buildId>:e:<key>`. Tags are not scanned: each entry records its tags, and one Redis hash holds the last revalidation time per tag. On read the handler compares the entry's `lastModified` with those times and treats an older entry as a miss.
- Tag times use each replica's clock. Keep replica clocks in sync (NTP); a few milliseconds of skew is harmless, seconds can delay or skip an invalidation.
- Fail open: any Redis error, timeout or missing URL switches that replica to a small in-process LRU shared by every cache instance in the process, so pages still cache per replica. It logs at most one line per minute to stderr per process and retries Redis after `LEVELRAIL_NEXT_CACHE_RETRY_MS`. Requests are never failed by the cache. While degraded, replicas do not share content. Revalidations made during the outage are remembered and written to Redis when it returns (before the first read), so an entry cached before the outage is not served stale; when Redis is back the fallback entries are dropped.
- The tag hash has no TTL (dropping it would lose invalidations) and grows by one small field per distinct tag.

## Next.js versions

The handler implements the `get`, `set`, `revalidateTag` and `resetRequestCache` shape used by Next 14 and 15, and also exposes `updateTags` for the Next 16 name. Differences handled: Next 14 passes tags in the entry or in the `x-next-cache-tags` header, Next 15 passes `ctx.tags`; extra `revalidateTag` arguments (Next 16 profiles) are ignored. Next 16 Cache Components (`'use cache'`) use a different interface (`cacheHandlers`), covered by the second export below.

## Cache Components (`'use cache'`, Next 16)

Add `cache-components-handler.js` next to `next.config.js` and register it for the `default` and, if you use `'use cache: remote'`, `remote` cache kinds:

```js
// cache-components-handler.js
module.exports = require('@levelrail/next-cache-handler').componentsHandler;
```

```js
// next.config.js (next to the cacheHandler settings above, which still serve ISR and fetch)
module.exports = {
  cacheComponents: true,
  cacheHandlers: {
    default: require.resolve('./cache-components-handler.js'),
    remote: require.resolve('./cache-components-handler.js'),
  },
};
```

It implements `get`, `set`, `refreshTags`, `getExpiration` and `updateTags` on the same Redis client, key prefix (`LEVELRAIL_NEXT_CACHE_PREFIX`, `NEXT_BUILD_ID`), TLS/CA settings, size limit, in-process fallback and outage replay as the classic handler, and the same tag hash, so `revalidateTag` and `updateTag` reach both. Entries are stored as `<prefix>:<buildId>:c:<key>`. The value stream is read fully in `set` and stored as one base64 payload (subject to `LEVELRAIL_NEXT_CACHE_MAX_VALUE_BYTES`), and `get` returns a fresh stream each time. The Redis TTL is the entry's `expire`. Entries are checked against the entry tags and the request's soft tags on every `get`, so `refreshTags` has nothing to sync and `getExpiration` returns the newest revalidation time of the tags it is given (0 if none).

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
- The Cache Components handler follows the `cacheHandlers` contract in the Next 16 docs but has not run under a real Next 16 `next start` or against a real Redis; verify streams and invalidation on the first deployment.
- Not verified against a real multi replica Next.js run. The unit tests use an in-memory fake of the Redis client; the interface shape follows the documented Next 14 and 15 `cacheHandler` contract and has not been exercised by a real `next start` yet. Treat the first deployment as the test.
