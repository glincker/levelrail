# Levelrail TypeScript SDK

Typed client for the Levelrail control plane API. No runtime dependencies, uses the built-in `fetch` (Node 20 or newer, Deno, Bun, browsers). The operation list and the namespaced methods are generated from `docs/public/openapi.json` by `scripts/gen-ts-sdk.mjs`. Request and response bodies are typed only where the OpenAPI file describes them, so most responses are `unknown` for now.

The package name `levelrail-sdk-placeholder` is a placeholder until the npm scope is chosen. The package is marked private and is not published.

## Setup

```ts
import { Levelrail } from "levelrail-sdk-placeholder";

const lr = new Levelrail({
  baseUrl: "https://control-plane.example.com", // or LEVELRAIL_API_URL
  token: process.env.LEVELRAIL_TOKEN,           // or set LEVELRAIL_TOKEN
});
```

Namespaces: `apps`, `deploys`, `staticSites`, `nodes`, `builders`, `environments`, `tokens`, `domains`. Every other operation is reachable with `lr.call("<operationId>", { path, query, body })`. Path parameters are required and typed from the route.

## Examples

### 1. Deploy an image

```ts
const idem = crypto.randomUUID();
await lr.apps.createApp({ body: { name: "web", image: "nginx:1.27", port: 80 } }, { idempotencyKey: idem });
const deploy = await lr.deploys.triggerDeploy({ path: { name: "web" }, body: {} }, { idempotencyKey: crypto.randomUUID() });
console.log(deploy);
```

### 2. Upload a Pages site

The control plane fetches an https `.zip` or `.tar.gz` and publishes it as the first deploy.

```ts
await lr.staticSites.createStaticSite({
  body: { name: "docs", domains: ["docs.example.com"], source_url: "https://files.example.com/site.zip" },
});
const deploys = await lr.staticSites.listStaticSiteDeploys({ path: { name: "docs" } });
```

### 3. List builders

```ts
const envs = await lr.environments.listAllEnvironments();
const builders = await lr.builders.getEnvironmentBuilders({ path: { id: "env_123" } });
```

### 4. Stream deploy status

```ts
const ctl = new AbortController();
for await (const ev of lr.deploys.deployLogStream({ path: { name: "web", deployId: "dep_123" } }, { signal: ctl.signal })) {
  console.log(ev.event, ev.data); // call ctl.abort() to stop early
}
```

### 5. Create a token

Token creation is session-only on the server, so run this with a session-authenticated deployment of the client. Listing works with a root token.

```ts
const created = await lr.tokens.createToken({ body: { name: "ci", abilities: ["read", "deploy"], expires_in_days: 30 } });
const all = await lr.tokens.listTokens();
```

## Errors, retries, aborts

- Non-2xx responses throw `LevelrailApiError` with `status`, `code`, `message`, `requestId` and `fix` from the API error body. Connection failures throw `LevelrailNetworkError`.
- `429` is always retried. `500`, `502`, `503` and `504` are retried for GET, PUT and DELETE, or for any request that carries an `idempotencyKey`. Backoff is exponential with jitter and honors `Retry-After`.
- Pass `signal` in the request options to abort, including during a retry wait or a stream.
- Tunables: `LEVELRAIL_SDK_MAX_RETRIES`, `LEVELRAIL_SDK_RETRY_BASE_MS`, `LEVELRAIL_SDK_RETRY_MAX_MS`, `LEVELRAIL_SDK_TIMEOUT_MS`, `LEVELRAIL_SDK_PAGE_SIZE`, or the matching client options.

## Pagination

```ts
for await (const app of lr.paginate("handleListApps", undefined, { pageSize: 50 })) console.log(app);
const everything = await lr.listAll("handleListApps");
```

## Regenerating

```sh
node scripts/gen-ts-sdk.mjs          # rewrite generated files
node scripts/gen-ts-sdk.mjs --check  # fail when they are stale
```
