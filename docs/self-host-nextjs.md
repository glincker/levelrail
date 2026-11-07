---
title: "Self-host Next.js on your own server with TLS and rollback"
description: "How to self-host a Next.js app with Levelrail: Dockerfile build, automatic HTTPS, preview environments per pull request and env vars."
"og:url": "https://docs.levelrail.com/self-host-nextjs"
---

**Run Next.js on a server you own, without the glue scripts**

Build with the Next.js preset or your own Dockerfile, get HTTPS, preview environments per pull request, a managed database and one-click rollback, deployed on every push to your git host.

[Get started](/getting-started) | [Vercel migration runbook](/migrating-from-vercel)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## What you get for a Next.js app

<CardGroup cols={3}>
  <Card title="Build and deploy">
    Connect GitHub, GitLab, Bitbucket or Gitea. Pushes build a Dockerfile with BuildKit and cut traffic over only when the readiness probe passes.
  </Card>
  <Card title="Environment and build-time vars">
    Env vars are encrypted per app. NEXT_PUBLIC_* values are inlined at build time, so set them before the build, which the migration runbook covers.
  </Card>
  <Card title="Database next to the app">
    Create Postgres or Redis as a managed resource with scheduled, verified backups and link it to the app.
  </Card>
</CardGroup>

## What Levelrail covers, and what it does not

Each row says what Levelrail does today. Rows marked "not yet verified on a real host" are documented behavior that has no recorded live run.

| | Managed platform | Levelrail on your server |
| --- | --- | --- |
| Git, build, deploy | Git integration | Webhook deploys, Railpack or your Dockerfile, cutover after the readiness probe passes. See [Next.js](/nextjs) |
| Domains and DNS | Managed | Add a domain with `levelrail-cli apps domains add`, with DNS records created through a connected Cloudflare or Route 53 account. See [Domains and ingress](/domains-and-ingress) |
| Edge cache | Global edge network | One response cache in the ingress, with rules and purge by URL, prefix, tag or all. See [CDN and caching](/cdn-and-caching). It is a single cache, not a multi-region one |
| CDN in front | Built in | Levelrail connects to Cloudflare as the DNS provider, with a per-app proxy switch (`levelrail-cli apps cdn set <app> --proxied on`) and an automatic cache purge on deploy and rollback. Other CDNs can sit in front, but you purge them with their own tools. See [Cloudflare in front](/cloudflare-in-front) |
| Functions | Serverless functions | [Functions](/functions) run as container apps that sleep when idle and wake on the first request, with triggers, a key-value store, versions and replay. Long-lived containers are the default for the app itself, and [sleep when idle](/sleep-when-idle) gives scale to zero. Cold start is a container start, and there is no edge runtime |
| Image optimization | Built in | `next/image` runs in your container. A [persistent image cache volume](/nextjs-image-cache) keeps results across deploys. There is no separate resizing service |
| ISR and revalidation | Built in | Runs inside your app. The cache survives redeploys with a volume, and [Redis shared cache](/nextjs-shared-cache) keeps replicas in step. Scheduled revalidation outside the app is not yet offered |
| Skew protection | Built in | The previous release is held for a grace window after a cutover. See [skew protection](/nextjs-skew-protection). Not yet verified on a real host |
| Previews | Per pull request | Per pull request, torn down on close, with optional basic auth. See [Preview environments](/preview-environments) |
| Rollback | Instant | Back to the pinned image of a past deploy, no rebuild. See [Roll back](/deploying-apps#roll-back) |
| Cost model | Usage and seat pricing | The software is free under Apache 2.0. You pay for the servers you run it on |

## Deploy a Next.js app in about 10 minutes

The time depends mostly on your first build. Plan for the install and build of your own project, then a few minutes for the rest.

<Steps>
  <Step title="Detect and create the app">
    Connect the repository. When `next` is in `package.json`, the Next.js preset is picked: port 3000, a Railpack build and a start command based on your `output` setting. Check the proposed start command before you create the app. Details are in [Next.js](/nextjs#what-is-detected). Use your own Dockerfile if you want a smaller image or run a monorepo.
  </Step>
  <Step title="Set build-time variables, then deploy">
    NEXT_PUBLIC_* values are inlined at build time, so set them before the first build. Push to your branch. The new release takes traffic only after the readiness probe passes.
  </Step>
  <Step title="Add your domain">
    `levelrail-cli apps domains add web app.example.com`. Point DNS at the server, or connect Cloudflare as the DNS provider and Levelrail creates the record. See [Domains and ingress](/domains-and-ingress).
  </Step>
  <Step title="Put Cloudflare in front">
    Follow the ordered checklist in [Cloudflare in front](/cloudflare-in-front): SSL mode, client address preset, then the per-app proxy. Deploys and rollbacks purge the Cloudflare cache for the app's hostnames. This was not yet verified on a live proxied record.
  </Step>
  <Step title="Open a pull request">
    `levelrail-cli apps previews enable web`. Each pull request gets a preview that is torn down when it closes. See [Preview environments](/preview-environments).
  </Step>
  <Step title="Practice a rollback">
    `levelrail-cli apps deploys rollback-to web <deploy-id>` puts the exact image of an earlier deploy back without a rebuild. The dashboard has a Rollback button per deploy.
  </Step>
</Steps>

## What to expect for build time and cache

- The first build is the slowest. Later builds reuse BuildKit cache steps, and the Deploys page shows median build time and cache hit rate. See [Build performance](/build-performance).
- Next.js writes its page and image cache under `.next/cache`. Without a volume, each deploy starts cold. The Next.js preset mounts a volume for it, and `image_cache: persistent` keeps optimized images. See [Next.js](/nextjs#keep-the-cache-across-deploys) and [image cache](/nextjs-image-cache).
- With more than one replica, each keeps its own ISR cache unless you use the [Redis shared cache](/nextjs-shared-cache).
- Turning on the ingress cache is optional and starts with the "Cache static assets" preset in [CDN and caching](/cdn-and-caching#turning-it-on). The ingress never caches requests that carry cookies or an Authorization header unless a rule allows it.
- A single small VPS is the best tested path. Build memory counts, so check [Build performance](/build-performance#sizing-a-vps).

## Troubleshooting

- [Troubleshooting by symptom](/troubleshooting-by-symptom): crashloops, failed health checks, certificate and DNS problems.
- [Troubleshooting](/troubleshooting): the full list and what each doctor finding means.
- [Behind Cloudflare](/behind-cloudflare): redirect loops, 526 and 413 errors.
- [Next.js](/nextjs#health-check): health check and shutdown behavior.
- Run `levelrail-cli doctor` and `levelrail-cli attention` first.

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Does it support the App Router and server components?">
    It runs your container, so anything that runs in a Node container works. Levelrail does not run a separate Next.js-specific runtime.
  </Accordion>
  <Accordion title="How do preview environments work?">
    A pull request opened or updated deploys a preview. Closing the pull request tears it down, whether or not it merged.
  </Accordion>
  <Accordion title="Can I run it on one small VPS?">
    Yes. A single node is the best tested path.
  </Accordion>
  <Accordion title="Is there a migration guide?">
    Yes, a runbook for moving a Vercel-hosted Next.js app, with an inventory, a cutover plan and the gaps.
  </Accordion>
</AccordionGroup>

## Deploy your Next.js app to your own server

Start with a side project and move the rest when you trust it.

[Get started](/getting-started) | [Vercel migration runbook](/migrating-from-vercel)
