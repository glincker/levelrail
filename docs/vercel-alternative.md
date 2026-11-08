---
title: "Self-hosted Vercel alternative for Next.js and Docker apps"
description: "Levelrail is an open source, self-hosted alternative to Vercel's Git deploys: webhook push-to-deploy, TLS and preview environments per pull request."
"og:url": "https://docs.levelrail.com/vercel-alternative"
---

**Vercel-style deploys on servers you own**

Push to your git host and get a running app, preview environments per pull request, logs, metrics and one-click rollback, on your own Linux boxes. Deploys start from a forge webhook by default; a native `git push` remote is beta and off by default. HTTPS uses a self-signed certificate until you enable Let's Encrypt (see [Domains and ingress](/domains-and-ingress#tls-whats-actually-shipped-today)). One live Let's Encrypt run is recorded; renewal and DNS-01 wildcards are unverified.

[Get started](/getting-started) | [Migration guide](/migrating-from-vercel)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## The parts of Vercel people miss when they leave

<CardGroup cols={3}>
  <Card title="Preview environments per PR">
    GitHub, GitLab and Bitbucket webhooks create a preview for each pull request and tear it down when it closes. Stable on GitHub, beta on GitLab and Bitbucket.
  </Card>
  <Card title="One-click rollback">
    Prior images stay pinned, so going back means switching to an image that is still there, not rebuilding from an old commit.
  </Card>
  <Card title="Predictable cost">
    You pay for the servers you already rent, not per request or per seat. Metrics and logs stay on your own nodes.
  </Card>
</CardGroup>

## Where the trade-offs are

Vercel is a managed cloud with a global edge network. Levelrail is the other model: your servers, your data, your bill. It manages the pipeline from git to cache, and it connects to a CDN for global reach instead of running one.

| | Vercel | Levelrail |
| --- | --- | --- |
| Where it runs | A managed cloud you do not control | Linux servers you own, one or many |
| Deploys | Git integration, previews, instant rollback | Git integration, previews per PR, rollback with pinned images |
| Observability | See Vercel's own documentation | Metrics and log search built in, node-local |
| Data | Handled by the vendor | Secrets, metrics and logs stay on your nodes unless you configure an export or drain |
| Zero-downtime deploys | Atomic deployments | Cutover only after the readiness probe passes. Live-tested with zero failed requests on a single node; multi-node is beta and not measured |
| Env vars per environment | Production, Preview and Development | One env set per app, with preview and branch overrides. Instance-wide environments need the experimental `global-environments` flag |
| Functions | Serverless and edge functions | [Functions](/functions): container apps with triggers, a key-value store, versions, replay and local dev. They sleep when idle (scale to zero) and wake on the first request, or you can keep a container running. Cold start is a container start, and they run on your servers |
| Edge cache | Global edge network | One response cache in the ingress with rules and purge by URL, prefix, tag or all. See [CDN and caching](/cdn-and-caching). It is a single cache, not multi-region |
| CDN in front | Built in | Levelrail connects to Cloudflare as the DNS provider, with a per-app proxy switch and automatic cache purge on deploy and rollback, so git, build, deploy, DNS, cache and purge are managed from one place. `levelrail-cli cdn setup <app> --dry-run` then `levelrail-cli cdn setup <app>` run the checks and turn the proxy on, and `levelrail-cli cdn edge <app>` shows the proxy state, last purge and cache hit rate. See [Cloudflare in front](/cloudflare-in-front). Not yet verified on a live proxied record. Other CDNs can sit in front, and you purge them with their own tools |
| Edge runtime | Edge and middleware runtime | Not offered. Code runs as containers on your servers |
| ISR revalidation | Incremental static regeneration | Runs inside your Next.js app. The Next.js preset mounts a `next-cache` volume at `/app/.next/cache`, so ISR pages survive redeploys. Replicas can share it through the [Redis cache handler](/nextjs-shared-cache). Scheduled revalidation outside the app is not yet offered |
| Image optimization | Built-in resizing | Served by your app: `next/image` runs in your container and uses your node's CPU and memory. A [persistent image cache](/nextjs-image-cache) keeps results across deploys. Put a CDN in front for global delivery |
| Visitor analytics | Web Analytics | Not offered. Request rate, errors and latency percentiles per app are built in |
| Blob and KV storage | Blob, KV | Not offered. Managed Postgres and Redis, and S3-compatible storage for backups |

[Read the Vercel migration runbook](/migrating-from-vercel)

## From Vercel to your own server

<Steps>
  <Step title="Inventory what Vercel holds">
    Env vars per environment, domains, redirects and cron jobs live in the dashboard, not your repo. The runbook has the checklist.
  </Step>
  <Step title="Deploy to Levelrail">
    Connect the repo, set env vars, and deploy a Dockerfile build. Check it on a temporary domain.
  </Step>
  <Step title="Cut DNS over">
    Lower TTLs first, switch, and keep Vercel as a fallback until you trust the new setup.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Can Levelrail host Next.js?">
    Yes, as a container built from your Dockerfile. Build-time variables such as NEXT_PUBLIC_* are inlined at build, which the migration runbook covers.
  </Accordion>
  <Accordion title="Does it have serverless functions or an edge network?">
    Functions, yes: `levelrail-cli functions` deploys a handler as a container app that sleeps when idle and answers the first request after a cold start (one URL per function, no per-request concurrency control). See [Functions](/functions). There is no edge runtime, since code runs on your servers. For a global edge network, connect Cloudflare: Levelrail manages its DNS records, the per-app proxy and cache purge. `levelrail-cli cdn setup <app>` (after a `--dry-run`) runs the setup checks and turns the proxy on. See [Cloudflare in front](/cloudflare-in-front) and [CDN and caching](/cdn-and-caching).
  </Accordion>
  <Accordion title="Is there an importer for Vercel?">
    Yes, an offline one. `levelrail-cli import config` reads `vercel.json` (with a `Procfile` or `app.json` if present) and prints a proposed `app.yaml` with findings for what has no equivalent. It needs no token and changes nothing. The runbook maps each Vercel concept to its Levelrail equivalent and lists the gaps.
  </Accordion>
  <Accordion title="What does it cost?">
    The software is free under Apache 2.0. Your cost is the servers you run it on, rather than usage and seat pricing.
  </Accordion>
  <Accordion title="Does Levelrail support SSO, SAML and SCIM?">
    Levelrail has password, passkey and TOTP sign-in, OAuth sign-in with 20 providers and generic OpenID Connect, IAM policies, teams and a hash-chained audit log, all in the one free edition. SAML single sign-on and SCIM provisioning exist but are beta: they sit behind the `saml-scim` feature toggle, are off by default, and are tested against a test identity provider rather than a live Okta or Entra ID tenant. See [SAML and SCIM](/sso-saml).
  </Accordion>
</AccordionGroup>

## Related

- [Migrating from Vercel](/migrating-from-vercel)
- [Cloud Run alternative](/cloud-run-alternative)
- [Self-host Next.js](/self-host-nextjs)

## Own your deploy pipeline

Run a side project on it first. Open source, so you can read how every step works.

[Get started](/getting-started) | [Migration guide](/migrating-from-vercel)
