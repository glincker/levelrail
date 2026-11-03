---
title: "Deploy a Next.js app"
sidebarTitle: "Next.js"
description: "How to run a Next.js app on Levelrail today: the standalone output, build time variables, the cache volume, health checks, and what to know before you run more than one replica."
"og:url": "https://docs.levelrail.com/nextjs"
---

This page covers what works today, what you set yourself, and what is not solved yet. The preset table is in [Framework presets](/frameworks).

## What is detected

When the repository has `next` in `package.json` dependencies, the detector picks the Next.js preset: port 3000, health path `/`, a Railpack build, and a volume hint for `/app/.next/cache`. It also reads `next.config.js`, `.mjs`, `.ts` or `.cjs`:

| `output` in the config | Result |
| --- | --- |
| `'standalone'` | The suggested start command is `node .next/standalone/server.js`. |
| `'export'` | Treated as a static site. The `out` directory is served and no container runs. |
| not set | The suggested start command is `next start`. |

The detector shows its reasons in the create-app form. Check the start command it proposes before you create the app.

## Choose a build path

There are two ways to build, and you pick one per app.

- **Railpack (the preset).** No files to add. A plain app starts with `npm run start`. With `output: 'standalone'` (and no `outputFileTracingRoot`) the build copies `.next/static` and `public` into `.next/standalone`, starts `node .next/standalone/server.js` and sets `HOSTNAME=0.0.0.0`; `PORT` comes from the app port.
- **Your own Dockerfile.** Use this for a monorepo with a custom `outputFileTracingRoot`, or when you want the smallest image and a non-root user.

To print the Dockerfile the preset uses:

```bash
levelrail-cli build frameworks nextjs --dockerfile > Dockerfile
```

It is a three stage build on `node:22-alpine` with a non-root user. It expects `output: 'standalone'` in your config and a `build` script in `package.json`. It does not install `sharp`; see below.

## Images and sharp

Next.js uses `sharp` to optimize images in a production server. Add it as a dependency of your app (`npm install sharp`) so the standalone trace includes it. Without it Next.js falls back to a slower path and logs a warning. The generated Dockerfile does not add it for you. The arm64 case has not been verified on a real arm64 host.

## Keep the cache across deploys

The preset suggests a volume named `next-cache` at `/app/.next/cache`. It holds the image optimization cache and ISR pages, so they survive a redeploy. In `app.yaml`:

```yaml
services:
  web:
    volumes:
      - name: next-cache
        path: /app/.next/cache
```

With the `--framework nextjs` flag on `apps create`, the volume is attached for you if you did not set one.

The volume belongs to one container. See the replica section below.

## Health check

The preset uses `/` as the health path. A page that renders data can be slow or fail for reasons that do not mean the process is down, so add a cheap route and use it. In the App Router, create `app/api/health/route.ts`:

```ts
export function GET() {
  return new Response("ok");
}
```

Then point readiness at it:

```yaml
services:
  web:
    health:
      readiness: { path: /api/health, interval: 5s, timeout: 2s }
```

Readiness gates the cutover of a new deploy. See [Health checks](/app-spec-reference#health-checks).

## Shutdown and keep-alive

On a deploy, Levelrail sends `SIGTERM` to the old container and waits up to `APP_RECONCILE_STOP_GRACE` (default `10s`) before `SIGKILL`. If your pages take long to finish, raise it. See [Stop grace](/deploy-safety#stop-grace).

Node closes idle keep-alive connections after a few seconds by default, while the ingress keeps its own connections open for `APP_INGRESS_IDLE_TIMEOUT` (default `2m`). Recent standalone `server.js` files read `KEEP_ALIVE_TIMEOUT` (milliseconds) and apply it. Check the file in your build before you rely on it. This has not been tested against the ingress here.

## NEXT_PUBLIC variables are build time values

Next.js writes `NEXT_PUBLIC_*` values into the JavaScript at `next build`. Changing one needs a new build. A restart is not enough.

What Levelrail does:

- Plain (non-secret) app env variables whose names start with `NEXT_PUBLIC_` are passed to the build. The list of prefixes is `APP_STATIC_BUILD_ENV_PREFIXES`.
- On the Railpack route, the values are part of the build cache key, so a changed value does not reuse an old layer.
- With your own Dockerfile, a value is passed only if the Dockerfile declares `ARG NAME`. Add `ARG NEXT_PUBLIC_API_URL` before `RUN npm run build`. The Dockerfile printed by the preset does not declare any, so add the ones you use.
- Secrets are never passed to a build. Do not use a secret for a `NEXT_PUBLIC_` value; it would be public anyway.
- The build log has a `build env` line that names the keys passed, never the values.

Set the value on the app's Environment tab, or import it from a file, then deploy again:

```bash
echo 'NEXT_PUBLIC_API_URL=https://api.example.com' > public.env
levelrail-cli apps env import web --file public.env --all-plain --apply
```

## Running more than one replica

Several things are per instance today. Read this before you set `replicas` above 1.

**Server Actions key.** Next.js encrypts Server Action identifiers with a key it generates for each build. Instances that do not share a key can fail actions that were started on another instance. Set the same key on every replica. Generate one and store it as a secret:

```bash
openssl rand -base64 32
levelrail-cli apps secrets set web NEXT_SERVER_ACTIONS_ENCRYPTION_KEY '<value>' --apply
```

Levelrail does not generate or set this key for you today. Next.js documents the key as a build time value as well, and Levelrail does not pass secrets to a build. Whether a runtime-only key is enough for your Next.js version is not verified here, so test an action across two replicas before you rely on it.

**ISR and the data cache.** The ISR pages, the fetch data cache and the image cache are stored on the container's disk. With several replicas, each one has its own copy. `revalidatePath` and `revalidateTag` act on the instance that handled the call, so other replicas can keep serving the old page until they revalidate themselves. The `next-cache` volume does not change this: a named volume is scoped to one service on one node.

The usual fix is a shared cache handler: set the `cacheHandler` option in `next.config` to a module that reads and writes the cache in a shared store such as Redis, and set `cacheMaxMemorySize: 0` so the in-memory copy does not shadow it. You write or choose that module yourself.

What Levelrail does today: it can run a Redis database next to the app and give you its connection string. What it does not do: provide or configure a cache handler, or share `.next/cache` between replicas.

If you do not need several replicas, run one. A single instance has none of these problems.

## Related

- [Framework presets](/frameworks)
- [App spec reference](/app-spec-reference)
- [Environment variables and secrets](/environment-and-secrets)
- [Build performance](/build-performance)
- [Self-host Next.js](/self-host-nextjs)
