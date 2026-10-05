---
description: A practical runbook for moving a Vercel-hosted Next.js app to Levelrail, with an inventory, a concept mapping table, a DNS cutover and rollback plan, the gaps that have no equivalent, and a shadow-run checklist.
---

# Migrating a Next.js app from Vercel

This is a runbook for moving one Vercel-hosted Next.js app onto a Levelrail instance you run yourself. It maps only to features that exist in this repository today. Where something is unverified or has no equivalent, it says so.

::: warning
Levelrail has no stable release yet (see [feature status](feature-status.md)), and real public ACME has one recorded live run but no renewal or wildcard verification yet (see [acme-verification-runbook.md](acme-verification-runbook.md)). Run the new deployment in parallel with Vercel, and keep Vercel as the rollback path until the shadow run in section 6 is clean.
:::

Unlike the platform importer in [Migrating from Coolify, Dokploy, or CapRover](migrating-from-coolify-dokploy-and-caprover.md), there is no importer for Vercel. Everything below is manual. Record every surprise you hit in `FRICTION.md` at the repo root.

## 1. Inventory what Vercel holds

Collect this before touching Levelrail. Vercel keeps configuration in its dashboard that is not in your repo.

| Item | Where to find it in Vercel | Why you need it |
| --- | --- | --- |
| Env vars, per environment | Project Settings, Environment Variables. Note which are Production, Preview, or Development, and which are Sensitive. | Levelrail keeps one env set per app, so you decide how each Vercel environment maps. |
| `NEXT_PUBLIC_*` vars | Same list. | These are inlined at build time, not read at runtime. See the build-time note below. |
| Domains and redirects | Project Settings, Domains, including the apex, `www`, and any redirect between them. | You recreate each one and cut DNS over. |
| DNS provider and current TTLs | Your registrar or DNS host. | You lower TTLs before cutover. |
| Cron jobs | `vercel.json` `crons`, and the Cron Jobs tab. | Vercel cron calls an HTTP path. Levelrail scheduled tasks run a command inside the container. |
| Build settings | Project Settings, General: framework preset, build command, output directory, install command, Node version, root directory. | These drive the choice between a Dockerfile and Railpack. |
| Integrations | Storage (Postgres, KV, Blob), analytics, log drains, marketplace add-ons. | Each needs its own answer. Managed storage does not move with the app. |
| Functions config | `vercel.json` `functions`, and `export const runtime = "edge"` in your code. | Edge runtime has no equivalent, see section 4. |
| Rewrites, headers, redirects | `next.config.js` and `vercel.json`. | Those in `next.config.js` run inside Next itself and move with the app. Those only in `vercel.json` do not. |

## 2. Map Vercel concepts to Levelrail

| Vercel | Levelrail equivalent | Notes |
| --- | --- | --- |
| Project | An app, with one or more services | See [Deploying apps](deploying-apps.md). |
| Git deploy on push | A git source with `trigger_mode: push`, from a GitHub, GitLab, Bitbucket, or Gitea connection | Provider connections are set up in the dashboard only. Path filters (`--paths`, `--paths-ignore`) exist. See [Git integrations](git-integrations.md). |
| Build with framework preset | `build.type: railpack` (Node.js is a supported provider) or `build.type: dockerfile` | The "Deploy from git" wizard shows what Railpack detects before you build. Only Node.js, Go, Java (Spring Boot), and Python are accepted. |
| `output: "standalone"` with your own Dockerfile | `build.type: dockerfile`, with `build.args` for `ARG` values | Use this when you need control over the image. You own the Dockerfile. Build args are not stored with the app, see the build-time section below. |
| Static export (`output: "export"`) | `build.type: static`, served by the embedded Caddy with no container | Static runs no build step. It copies `build.path` (relative to the repo root, or to `build.baseDirectory`) from the commit as it is, so the exported `out/` directory must be committed. Otherwise build with `dockerfile` or `railpack`. `apps create` does not accept static; deploy it with `apps deploy-spec <app> --file app.yaml --repo-url URL --ref main`. The spec forbids `port`, `host_port`, `bind_address`, and `hooks`. |
| Production and Preview environments | Optional environments inside a project. Apps are tagged with one. Promote copies only the image tag to a sibling app in another environment | Env vars stay per app, so a staging and a production app each keep their own. See [Projects and organizations](projects-and-organizations.md) and [Promote details](deploying-apps.md#promote-details). |
| Environment variables | Plain env (`apps env import --file`), encrypted secrets (`apps secrets set --env-file`), or Vault references | Plain import skips keys that are already secrets. Export never includes secret values. |
| Sensitive env vars | `apps secrets set`, envelope-encrypted and write-only | Never returned by the API after save. |
| Shared env vars | Shared env scoped to a project, organization, or environment (`shared-env set`) | See [Projects and organizations](projects-and-organizations.md). |
| Preview deployments per pull request | Preview environments per PR, opt-in per app: `apps previews enable <app>`. Deployed as `<app>-pr-<number>`, torn down on close or merge | GitHub, GitLab, Bitbucket, and Gitea. The preview domain is `pr-<number>.<app>.<primary-domain>` and needs a control-plane primary domain. There is no per-branch preview URL. See [Git integrations](git-integrations.md#preview-environments-per-pull-request). |
| Preview-only env values | `apps preview-env set`, and branch-scoped `apps branch-env set` | Single-service previews only. |
| PR comment with preview URL | A PR comment and a `levelrail/preview` commit status, when `apps previews pr-status enable` is on | One comment per PR, kept up to date. |
| Deployment protection | Protected environments and deploy approvals | Deploy approvals are labeled stable in [feature status](feature-status.md). |
| Domains | `domains:` in the app spec, or `apps domains add`. Caddy routes by `Host` and handles TLS | Point an A or AAAA record at the server. See [Domains and ingress](domains-and-ingress.md). |
| `www` to apex redirect | Add `www` to the app's domains, then `domains redirect set <app> www.example.com --target https://example.com` (301 or 302) | The target must be an absolute URL. A bare origin target keeps the request path and query; a target with its own path always redirects there. |
| Automatic HTTPS | Caddy internal issuer by default (self-signed), or real ACME once enabled in settings | ACME is built and unit-tested but not verified against a real domain. Bring-your-own certificate is also supported. |
| Instant Rollback | `apps deploys rollback-to <app> <deploy-id>` (pinned by digest), `apps rollback <app> --image <tag>` for a tag listed by `apps images <app>`, the dashboard's per-deploy Rollback button, and auto-rollback on crashloop | Prior images are pinned so garbage collection cannot remove a rollback target. Take the deploy id from `apps deploys list <app>`. |
| Cron Jobs | Scheduled tasks: `apps scheduled-tasks create`, with a 5-field cron expression and an argv command run inside the running container | Not an HTTP call. To trigger a route, the command must use something present in your image. Confirm the tool exists in the image. |
| Runtime and build logs | `apps logs`, `apps deploys logs`, a node-local log store with full-text search, and live build steps | See [Observability](observability.md). |
| Firewall and rate limiting | Opt-in WAF (detect or block) and rate limiting per domain | See [Domains and ingress](domains-and-ingress.md). |
| Maintenance page | Per-domain maintenance mode | Takes precedence over a redirect. |
| Vercel Postgres, KV | Managed databases (Postgres, Redis, and others) with scheduled backups | You dump and restore the data yourself. See [Managing databases](managing-databases.md). |
| Team members and roles | IAM-style Allow and Deny policies and scoped tokens | Beta. See [Identity and access](identity-and-access.md). |

### Build-time versus runtime env

`NEXT_PUBLIC_*` values are compiled into the JavaScript bundle during `next build`. App env and secrets are only injected when the container is created, never into a build, so setting `NEXT_PUBLIC_*` with `apps env import` does not change the browser code. Verified on a Next.js 15 App Router app:

- **Any build type, including push deploys:** commit a `.env.production` holding the `NEXT_PUBLIC_*` values. `next build` reads it, so it works with Railpack and with a Dockerfile that copies the repo. These values end up in public JavaScript anyway, so committing them exposes nothing new. This is the recommended path.
- **With `build.type: railpack`:** there is no other way today. Railpack builds receive no build-time env, and `build.args` is rejected for Railpack. A value that is only set as app env stays undefined in the client bundle, and the server render shows a different value than the browser (a hydration mismatch).
- **With `build.type: dockerfile`:** `build.args` plus matching `ARG` and `ENV` lines in the Dockerfile also work, but build args are not stored with the app. `apps builds trigger` needs `--build-arg` on every call, and push deploys from a git source build without them, so the values silently become empty. Prefer `.env.production`, or give each `ARG` a default in the Dockerfile.

Railpack starts the app with `npm run start`, so `next start` serves it. With `output: "standalone"` Next.js logs a warning that `next start` does not use the standalone output; the app still serves normally. The standalone output only matters for your own Dockerfile.

Server-only variables are set as normal app env or secrets and read when the container is created. Changing them shows an "Environment changes pending restart" banner until you apply them (`apps apply <app>`).

## 3. Procedure

1. **Connect the git provider** in the dashboard, then confirm it from the CLI with `git-providers`.
2. **Create the app** from git, as in [Deploying apps, git-repo build](deploying-apps.md#_2-git-repo-build), or commit an `app.yaml`:

   ```yaml
   version: 1
   services:
     web:
       build:
         type: dockerfile
         path: ./Dockerfile
       port: 3000
       health:
         readiness: { path: /api/health, interval: 5s, timeout: 2s }
       env:
         NODE_ENV: production
         DATABASE_URL: { secret: true, required: true }
   ```

   Create it with `apps create --file app.yaml --repo URL --ref main --image-repo NAME --secret DATABASE_URL=...`. The first build starts as soon as the app exists, so a `{ secret: true, required: true }` variable must be given with `--secret` here; the CLI refuses to create the app without it. Without a Dockerfile use `--build-type railpack`, and pass the same `--build-type` to every later `apps builds trigger`, which does not remember it. `apps deploys wait <app>` follows the build.

   Leave out `domains:` for now, so nothing competes with Vercel for the real hostname. If `APP_PUBLIC_HOST` is set to a public IP, `apps network <app>` shows a zero-config sslip.io URL to test on. The host port behind it changes on every restart, so run `apps network` again after one.
3. **Load env vars.** Plain values with `apps env import <app> --file prod.env --dry-run`, then again without `--dry-run`. Secrets with `apps secrets set <app> --env-file secrets.env`. Copy from the Production column in Vercel, not Preview. Keep `NEXT_PUBLIC_*` out of these files; see the build-time section above. Run `apps apply <app>` afterwards.
4. **Add a readiness path.** Add a cheap route such as `/api/health` that does not depend on external services, and set it with `health.readiness` in `app.yaml` or `apps health set <app> --probe readiness --path /api/health`. Known issue: a deploy that fails readiness still receives traffic today (`FRICTION.md`, F-002). Check `apps status <app>` after each deploy and roll back with `apps deploys rollback-to` if it reports `RunningNotReady`.
5. **Deploy, then verify** on the zero-config URL: pages, API routes, auth callbacks, image requests, and streaming responses.
6. **Move data** if the app uses Vercel Postgres or KV: create a managed database here, restore a dump into it, and rehearse the restore before cutover.
7. **Recreate cron jobs** as scheduled tasks, and run each once with `apps scheduled-tasks run`.
8. **Enable previews** with `apps previews enable <app>` if you rely on PR preview URLs.

## 4. What has no equivalent

Plan for these before you commit to moving.

| Vercel feature | Status on Levelrail |
| --- | --- |
| Edge Functions and Edge Middleware runtime | None. Everything runs as a normal Node container. Middleware that only uses Node-compatible APIs works. Code that depends on the edge runtime needs to move to the Node runtime. |
| Image Optimization CDN | No global image CDN. `next/image` runs inside your container, so it uses your server's CPU and memory and is served from wherever your node is. Consider `sharp` in the image, or `images.unoptimized`. |
| Global CDN and edge caching | None built in. Caddy serves from the node or nodes you run. ISR and the Next.js data cache live on the container's filesystem, so use a volume if they must survive redeploys. Multiple replicas do not share that cache. |
| Web Analytics and Speed Insights (real-user Core Web Vitals) | No browser-side analytics. Server-side request metrics exist (rate, 4xx and 5xx, p50, p95, p99 per app). The [integrations](integrations.md) page lists third-party options such as PostHog. |
| Blob, KV, Edge Config | No drop-in equivalents. Managed Redis exists, but you rewrite the client code. |
| Per-branch preview URLs | Previews are per pull request only. |
| Serverless scale to zero and autoscaling | Long-running containers with fixed replicas. There is no autoscaling. |
| Vercel Cron calling HTTP routes | Scheduled tasks exec a command in the container instead. |
| HTTP to HTTPS redirect | None. The ingress does not redirect `http://` to `https://`, so plain HTTP links to your domain fail. Put the redirect in front (for example at your DNS or CDN provider) if you rely on it. |
| Build-time env for Railpack | None. Use a committed `.env.production`, see the build-time section in 2. |

## 5. DNS cutover and rollback plan

1. **Lower TTLs first.** At least one full old-TTL period before cutover (often 24 hours), set the TTL on the relevant A, AAAA, and CNAME records to 60 to 300 seconds.
2. **Add the real domains** to the app, including `www` if you redirect it: `apps domains add <app> app.example.com www.example.com`, or `domains:` in `app.yaml` and a redeploy. Then set the redirect, `domains redirect set <app> www.example.com --target https://app.example.com`. Check certificate state on the Domains tab.
3. **Get a certificate ready before traffic arrives.** With ACME enabled, issuance needs the domain to already point at the server, so expect a short window. To avoid it, upload a bring-your-own certificate for the domain first, or rehearse ACME on a spare hostname using the [runbook](acme-verification-runbook.md).
4. **Test with a host override** before changing DNS, for example `curl --resolve app.example.com:443:<server-ip> https://app.example.com/` (add `-k` while the certificate is self-signed). Check the redirect the same way on `www.example.com`.
5. **Cut over.** Change the A and AAAA records for the apex and `www` from Vercel to the Levelrail server's public IP. Do not remove the domain from Vercel until the shadow run is done.
6. **Watch.** Run `dig +short app.example.com` from outside your network, then check the Metrics tab and `apps logs <app>`.
7. **Rollback plan.** Write down the old record values before changing them, and keep the Vercel project and its domain configuration untouched. To roll back, restore the old records. With the low TTL, most clients return within minutes. A bad Levelrail release is a separate case: use `apps deploys rollback-to <app> <deploy-id>` or the dashboard button.
8. **Raise TTLs again** only after the shadow run is clean.

## 6. Shadow-run checklist

Run both platforms side by side long enough for boring problems to appear. Suggested minimum: one full business cycle, including any weekly or monthly cron.

- [ ] Readiness probe passes, and a deliberately broken deploy is rejected while the old container keeps serving. This currently fails (F-002); until it is fixed, confirm you can detect it with `apps status` and recover with `apps deploys rollback-to`.
- [ ] The `www` redirect, basic auth, WAF, and uploaded certificates are still set after an env change and a redeploy.
- [ ] A rollback to the previous image works, from both the CLI and the dashboard.
- [ ] Every env var key present in Vercel Production exists here.
- [ ] Build-time `NEXT_PUBLIC_*` values are correct in the deployed bundle.
- [ ] Auth flows work: OAuth callback URLs updated at each provider, cookies set for the right domain.
- [ ] Third-party webhooks (payments and similar) point at the new URL, and a test event succeeds.
- [ ] Each scheduled task ran on schedule (check `last_run_status` and `consecutive_failures`).
- [ ] The certificate was issued and is not stuck in renewal, and certificate expiry alerting is configured.
- [ ] Alert rules and a notification channel exist for crashloops, disk space, and certificate expiry.
- [ ] A database backup ran, and a restore was actually rehearsed.
- [ ] Image optimization, ISR, and file uploads behave acceptably under real traffic.
- [ ] Response time and error rate compare acceptably against Vercel, using the app's request metrics.
- [ ] Preview environments open, update, and tear down on a test pull request.
- [ ] A control plane backup exists (see [Control plane backup](control-plane-backup.md)).
- [ ] Every issue found is logged in `FRICTION.md` with a severity.

Decommission Vercel only after this passes and you have decided what to do about each gap in section 4.

## See also

- [Deploying and managing apps](deploying-apps.md)
- [Domains and ingress](domains-and-ingress.md)
- [Git integrations](git-integrations.md)
- [app.yaml reference](app-spec-reference.md)
- [Feature status](feature-status.md)
