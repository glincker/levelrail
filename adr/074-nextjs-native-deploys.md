# ADR 074: Next.js native deploys

Status: Proposed

Date: 2026-11-06

## Context

A Next.js app on Levelrail is built by Railpack and run as one generic Node
container with a blue-green or rolling cutover. That works, but it leaves
the behaviour a Next.js app expects from its host to the user: build caches,
a shared ISR cache across replicas, the image optimizer cache, asset skew
during cutover, build time env, streaming. The goal of this ADR is that a
Next.js deploy on Levelrail behaves better in practice than on a plain
container host, judged on measured behaviour, not on feature names.

Every claim about current behaviour below cites the file it was read in.
Anything not read is listed under open questions, not stated as fact.

## What exists today

| Area | Behaviour | File |
| --- | --- | --- |
| Next.js detection | `package.json` with `next` in dependencies or devDependencies marks the build as `nextjs` | `internal/build/memguard.go` (`DetectBuildFramework`) |
| Memory warning | A host under the framework floor (2048 MiB for Next.js, documented guidance, not measured) gets a one line warning in the build log, at build start | `internal/build/memguard.go`, `internal/build/limits.go` |
| OOM message | Exit 137 is explained with the cap, host memory and options | `internal/build/memguard.go` (`MemoryKillMessage`) |
| Standalone output | `output: 'standalone'` in `next.config.*` switches the start command to `node .next/standalone/server.js`, copies `.next/static` and `public`, sets `HOSTNAME=0.0.0.0`. A custom `outputFileTracingRoot` or `distDir` opts out | `internal/build/railpack_nextjs.go` |
| Build cache mounts | Railpack v0.43.0 declares cache mounts for `.next/cache`, `node_modules`, turbo and the package manager store | Railpack `core/providers/node/node.go`, `package_manager.go` |
| Cache store scope | Levelrail builds the Railpack graph with `NewBuildKitCacheStore("")`, an empty unique id, so cache mount keys are not scoped per app | `internal/build/railpack.go` (around line 305) |
| BuildKit state | Daemon state and local cache live in the `levelrail-buildkit-state` volume, GC keep 10240 MB for 168h by default | `internal/build/buildkitd/config.go` |
| Remote layer cache | Object storage BuildKit cache, documented as for Dockerfile builds; failure downgrades to no cache | `internal/deploy/build_cache.go`, `internal/objectstore/buildcache.go` |
| Build env | Only literal, non secret env vars with a public prefix (`NEXT_PUBLIC_` and others, `APP_` seedable list) reach a build; key names are logged, never values | `internal/deploy/static_build_env.go`, `internal/deploy/build_env.go` |
| Env UI | The env editor knows the `NEXT_PUBLIC_` prefix | `web/src/lib/envRows.ts` |
| Rollout | Blue-green proves the new set healthy before the old set is torn down; recreate also exists | `internal/reconcile/application/controller.go` |
| Previous release hold | `deploy.previous_release_hold` runtime setting, default 3m, env seed `APP_DEPLOY_PREVIOUS_RELEASE_HOLD`, keeps old containers as instant rollback target and for draining routes | `internal/runtimesettings/registry_deploy.go`, `internal/reconcile/application/image_truth.go` |
| Rollback | Previous known good image is taken from deploy attempts | `internal/deploy/rollback.go` |
| Ingress | Caddy `reverse_proxy` with selection policies including `cookie` and `uri_hash`; no `flush_interval` field in the handler struct | `internal/ingress/config.go`, `internal/ingress/loadbalancer.go` |
| Edge cache | `levelrail_cache` handler streams to the client while keeping a copy up to a max, forwards `Flush` | `internal/ingress/policy_cache.go` |
| Previews | PR preview environments for several git providers with per PR env overrides, sweep, teardown, comments | `internal/api/preview_environments*.go`, `internal/api/preview_pr_env.go` |
| Cron | `jobs:` in app.yaml with schedule, timezone, mode fresh or exec, concurrency, timeout | `internal/spec/spec.go` (`Job`) |
| Volumes | Named volume or host path, no size field | `internal/spec/spec.go` (`Volume`) |
| Redis | Managed Redis family engines exist | `internal/store/database.go` |
| Platform import | `levelrail-cli import <platform>` reads a hosted platform account (projects, env keys, domains, GET only); config translation reads the project config file (crons, rewrites, redirects, headers) and appends a cutover checklist; SDK imports for hosted analytics are flagged | `cmd/levelrail-cli/import_platform.go`, `internal/platformimport/vercel.go`, `internal/configtranslate/vercel*.go` |
| Web vitals | No endpoint or card found in the code read | none |

## Decision

All new tunables are runtime settings (API, CLI, dashboard, applied live).
Env vars only seed defaults, prefixed `APP_`. Every feature is off by
default until its verification step passed on a real host.

### 1. Detection

Extend `DetectBuildFramework` into a small Next.js profile: resolved `next`
version (from the lockfile, falling back to the range in `package.json`),
output mode (standalone, export, default), router (app, pages), and whether
`images.unoptimized` is set. The profile is stored on the deploy attempt and
shown in the build log before the build starts. The memory warning moves
before the solve, not only into the log stream, and the wizard shows it.
Floors stay settings (`builds.nextjs_memory_floor_mb`, seed
`APP_BUILD_NEXTJS_MEMORY_FLOOR_MB`) and are labelled as guidance until a
measured number replaces them. Non standalone apps get a hint, never a block.
Trade off: regex reading of `next.config.*` misses computed configs, so the
profile says "unknown" instead of guessing.

### 2. Build cache that survives deploys

The mounts already exist (Railpack) and the state volume persists
(buildkitd config). Decision: pass the app id as the cache store unique id
so `.next/cache` and package caches are per app, then measure. Add a cache
hit line per mount to the build log and a "clear build cache" action per
app. Multi node builders do not share cache mounts (layer export does not
carry them), so pin an app's builds to the node that last built it when
more than one builder exists. Trade off: per app keys lower hit rate for
monorepos that share lockfiles; correctness and isolation win. The "about a
minute instead of 8" target is a hypothesis until slice S2b measures it.

### 3. Shared ISR and data cache across replicas

Ship a Levelrail cache handler package (a small npm module, versioned with
the platform) that implements the Next.js `cacheHandler` interface on a
managed Redis database, wired by env (`LEVELRAIL_NEXT_CACHE_URL`). Opt in
per app in app.yaml (`next.cache: redis`). With one replica nothing changes.
On demand revalidation then reaches every replica because tags live in
Redis. Rejected: a filesystem cache on a shared volume (locking, cross node
volumes). Trade off: a Redis outage must fall back to the in memory cache,
never fail the request; the handler is user facing code we have to maintain
per Next.js major.

### 4. Persistent image optimization cache

Mount a per app named volume at `.next/cache/images` (path adjusted for the
standalone layout) when the profile shows the optimizer is in use. Size cap
and pruning are settings (`next.image_cache_max_mb`, seed
`APP_NEXT_IMAGE_CACHE_MAX_MB`; prune interval
`APP_NEXT_IMAGE_CACHE_PRUNE_INTERVAL`) enforced by a janitor that deletes
oldest files first. Spec volumes have no size field today
(`internal/spec/spec.go`), so the cap is enforced by pruning, not quota.
Trade off: replicas on different nodes keep separate caches; acceptable.

### 5. Skew protection

Set `NEXT_DEPLOYMENT_ID` at build time to the deploy attempt id so the
client sends it on navigation. At the ingress, route requests carrying an
old deployment id to the held previous release while it exists, and keep
old `/_next/static` assets servable for a grace window
(`deploy.asset_grace`, seed `APP_DEPLOY_ASSET_GRACE`) by copying them out of
the image into a static release directory, reusing the static release
retention logic (`internal/deploy/static_retention.go`). Rollback keeps the
same rule in reverse. Trade off: routing by deployment id needs the
previous release containers alive, so the grace window cannot exceed
`deploy.previous_release_hold`; assets alone can outlive it.

### 6. Preview deployments

Already exist (`internal/api/preview_environments*.go`). Next.js work is
limited to: preview builds get their own `NEXT_PUBLIC_` values from the PR
env overrides, their own build cache key, no shared ISR namespace with
production (Redis key prefix per preview), and the preview URL injected as
a build time public var on request. No new preview system.

### 7. Web vitals

A same origin endpoint on the ingress (`/_levelrail/vitals`, path a setting)
accepts `navigator.sendBeacon` payloads from `useReportWebVitals`, rate
limited and size capped (`APP_VITALS_MAX_BODY_BYTES`,
`APP_VITALS_RATE_PER_MIN`), samples (`APP_VITALS_SAMPLE_PERCENT`), stores
p75 per route per deploy, and a dashboard card shows LCP, INP, CLS, TTFB
per deploy so a regression is tied to a release. No cookies, no IP stored.
Trade off: the app must add a ten line snippet; the import report already
points hosted analytics users here (`internal/configtranslate/vercel.go`).

### 8. Cron

`jobs:` already exists in app.yaml (`internal/spec/spec.go`) and the
translator already emits loopback HTTP cron calls with a bearer header from
`CRON_SECRET` (`internal/configtranslate/vercel_cron.go`). Decision: add an
`http` job form (`path:` instead of `command:`) that the scheduler calls
over loopback with the secret header, so users do not hand write curl.
Nothing else new.

### 9. Env model

`NEXT_PUBLIC_` values are inlined at build time. The env editor, CLI
`env set` and API response mark public prefix keys as "build time, needs a
rebuild", and saving one sets a pending rebuild state on the app with a
one click rebuild (restart is not offered as the fix). A public prefix key
marked secret is rejected with an explanation, matching the existing rule
that secrets never reach a build (`internal/deploy/static_build_env.go`).

### 10. Streaming and PPR

Set `flush_interval: -1` on reverse proxy routes of apps whose profile is
Next.js (setting `ingress.nextjs_flush_immediate`, default on once
verified), and confirm no handler in the chain buffers a streamed body. The
edge cache recorder already forwards `Flush` (`internal/ingress/policy_cache.go`).
Test plan: a fixture app with a Suspense boundary that sleeps 2s; assert
the first chunk reaches a raw client before the sleep ends, through plain
proxy, with compression, with the edge cache on, with an auth gate, and
over HTTP/2 and HTTP/1.1.

### Guided platform import

Extend the existing `levelrail-cli import` hosted platform source
(`cmd/levelrail-cli/import_platform.go`, `internal/platformimport/vercel.go`)
and config translation (`internal/configtranslate/vercel*.go`) into one
guided flow: read the project config file and the account, create the app
with the Next.js profile, set env (secrets as secrets, public prefix keys as
build time), add domains as pending, translate crons into `jobs:`, report
redirects and headers that must move into `next.config.*`, then print the
existing cutover checklist (`vercel_cutover.go`) with live checks (temporary
URL serves, certificate issued, TTL lowered). DNS is never changed by the
command.

## Alternatives rejected

- Running `next start` behind a custom Node adapter we own: more control,
  but ties every Next.js upgrade to our release.
- An edge runtime emulator: out of scope, edge files keep running as Node
  (`internal/configtranslate/vercel_edge.go` already warns).
- A shared volume ISR cache: locking and cross node problems, see 3.

## Security

- Build caches keyed per app (2); preview builds never reuse production
  caches.
- Cache handler credentials come from the managed database binding, never
  baked into the image.
- The vitals endpoint takes untrusted input: size, rate and schema limits,
  no PII stored.
- Cron HTTP jobs call loopback only; the secret is read at run time.

## Failure modes

- Redis down: handler falls back to in memory, logs once per minute.
- Image cache volume full: pruning runs early; the optimizer still works.
- Grace window shorter than real client sessions: hard reload, same as today.
- Wrong profile detection: features stay off, build proceeds as today.

## Cannot be verified locally

Repeat build time on a 2 vCPU host, memory floors, multi replica ISR
consistency, multi node cache behaviour and streaming through a real
public HTTPS path all need a real host run.

## Slices (dependency ordered, 15 to 25 minutes each)

| Slice | Work | Files | Depends on |
| --- | --- | --- | --- |
| S1 | Next.js profile type and detection | `internal/build/nextprofile.go` (new), its test | none |
| S2a | Per app cache store id, cache hit log | `internal/build/railpack.go`, `internal/build/railpack_test.go` | none |
| S2b | Clear build cache action (API plus CLI) | `internal/api/apps_build_cache.go` (new), `cmd/levelrail-cli/build_cache.go` (new) | S2a |
| S3 | Profile on deploy attempt, early memory warning | `internal/build/limits.go`, `internal/deploy/deploy.go` | S1 |
| S4 | Env build time marking and pending rebuild | `web/src/lib/envRows.ts`, `internal/api/env_settings.go` | none |
| S5 | Flush interval field and Next.js route wiring | `internal/ingress/config.go`, `internal/ingress/routes.go` | S1 |
| S6 | Streaming fixture and test | `test/nextjs-stream/` (new) | S5 |
| S7 | HTTP job form in spec and scheduler | `internal/spec/spec.go`, `internal/spec/validate_jobs.go`, `internal/scheduledtask/runner.go` | none |
| S8 | Image cache volume and pruning janitor | `internal/reconcile/application/next_image_cache.go` (new), `internal/janitor/next_image_cache.go` (new) | S1, S3 |
| S9 | `NEXT_DEPLOYMENT_ID` at build time | `internal/deploy/static_build_env.go` | S3 |
| S10 | Old static asset extraction and grace retention | `internal/deploy/next_assets.go` (new) | S9 |
| S11 | Ingress routing by deployment id | `internal/ingress/next_skew.go` (new) | S10 |
| S12 | Cache handler npm package | `kit/next-cache-handler/` (new) | none |
| S13 | `next.cache: redis` spec option and binding env | `internal/spec/nextjs.go` (new), `internal/reconcile/application/next_cache_env.go` (new) | S12 |
| S14 | Preview isolation (cache key, Redis prefix) | `internal/api/preview_lifecycle_deploy.go` | S2a, S13 |
| S15 | Vitals ingest endpoint and store | `internal/ingress/vitals.go` (new), `internal/store/vitals.go` (new) | none |
| S16 | Vitals dashboard card | `web/src/components/apps/VitalsCard.tsx` (new) | S15 |
| S17 | Guided import flow | `cmd/levelrail-cli/import_guided.go` (new), `internal/configtranslate/vercel_cutover.go` | S1, S4, S7 |
| S18 | Real host verification run and numbers into this ADR | this ADR only | all |

Each slice owns its files; S3 and S9 touch deploy code in sequence, never
in parallel. Exact file names for new files are proposals.

## Out of scope

Edge runtime and middleware at the edge, a global CDN, multi region,
serverless functions, hosted image optimization outside the app, automatic
DNS changes during import, and any change to how non Next.js apps build.

## Consequences

Next.js apps gain behaviour a plain container host does not give them,
at the cost of a maintained cache handler package and a framework profile
we must keep current with Next.js majors.

## Open questions

1. Does Caddy flush streamed responses without `flush_interval` today, and
   does any Levelrail handler (compression, auth gate, hardening) buffer?
2. Do Railpack cache mounts actually hit across deploys on one host today,
   and what is the measured repeat build time on 2 vCPU?
3. Does the standalone server read the image cache from
   `.next/standalone/.next/cache` or the app root, for each supported major?
4. Is the previous release hold long enough for skew routing, or does
   skew protection need its own longer container hold?
5. Is the remote object storage cache used for Railpack builds at all, or
   only Dockerfile builds as its doc comment says?
6. Which Next.js majors does the cache handler support at launch?
7. Should vitals live on the ingress or the control plane API?
8. Where should the pending rebuild state live: applied config tracking in
   the reconciler or the app record?
