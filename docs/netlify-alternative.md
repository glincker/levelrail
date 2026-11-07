---
title: "Self-hosted Netlify alternative for static sites and Docker apps"
description: "Levelrail is an open source, self-hosted Netlify alternative for static sites: build from git, _redirects and _headers, branch deploys and forms."
"og:url": "https://docs.levelrail.com/netlify-alternative"
---

**Netlify-style static deploys on a server you own**

Connect a repository, set a build command and a publish directory, and Levelrail builds the site and serves it from its own ingress. `_redirects` and `_headers` files work as written. Branches get stable URLs, forms land in an inbox, and a secret URL rebuilds the site when your CMS publishes.

[Get started](/getting-started) | [Migration guide](/migrating-from-netlify)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## What carries over

<CardGroup cols={3}>
  <Card title="Static sites from git">
    A build command and a publish directory, run in a throwaway container. The published files are kept per deploy, so a rollback does not rebuild. See [Static pages](/pages).
  </Card>
  <Card title="Your routing files">
    Redirects, rewrites, headers, single page app fallback, clean URLs and a custom 404 page, read from `_redirects`, `_headers` or `app.yaml`. See [Static routing](/static-routing).
  </Card>
  <Card title="Forms, hooks and jobs">
    Forms with an inbox and a spam trap, build hooks, scheduled calls to your app, and a publish lock. Forms are beta, so read each page's limits.
  </Card>
</CardGroup>

## When to choose it, and when not to

Choose Levelrail if you already rent a server, you run more than static sites (databases, containers, workers) and want one place for all of it, or you want deploy data and logs to stay on your own machines. The software is free under Apache 2.0. Your cost is the servers.

Stay on Netlify if you need Edge Functions, Netlify Identity or built-in analytics, which Levelrail does not offer. For global delivery, Levelrail connects to Cloudflare and manages the DNS records, proxy switch and cache purge, but it does not run its own multi-region network. The gaps are listed in the table below and in the guide.

## Comparison

| | Netlify | Levelrail |
| --- | --- | --- |
| Where it runs | A managed platform | Linux servers you own, one or many |
| Static build and publish | Build command and publish directory | The same two settings, built in a Node based container (`node:22-bookworm-slim` by default). Other toolchains need a custom build image |
| `_redirects` and `_headers` | Supported | Supported in Netlify syntax. Country, language, role and cookie conditions are skipped and reported. A `200` rewrite must point at a path on the same site |
| Deploy contexts | Production, deploy preview, branch deploy, per branch | `deploy-preview`, `branch-deploy` and `branch:<pattern>`, with env and static build overrides. See [Deploy contexts](/deploy-contexts) |
| Branch deploys | Per branch URL | Per branch URL, off by default, capped per app. See [Branch deploys](/preview-environments#branch-deploys) |
| Forms | Detected from your HTML | Created explicitly, with a public endpoint, inbox, CSV export and notifications. Beta. See [Forms](/forms) |
| Build hooks | Supported | Supported, with a secret URL per hook. See [Build hooks](/build-hooks) |
| Scheduled functions | Functions on a schedule | A cron call to a path on a container app. See [Scheduled functions](/scheduled-functions) |
| Lock deploys | Lock publishing | Publish lock: builds finish and wait as held builds. See [Publish lock](/publish-lock) |
| Split testing | Branch based | Container apps with a domain only. Static sites are not supported yet. See [Split testing](/split-testing) |
| Drop deploy | Drag a folder | Drag a folder or a zip, or `levelrail-cli deploy ./site --static`. See [Drop deploy](/drop-deploy) |
| Serverless functions | Netlify Functions | [Functions](/functions): container apps with triggers, a key-value store, versions and replay. They sleep when idle and wake on the first request, or stay running. Cold start is a container start |
| Edge Functions | Supported | Not offered, and a stated non-goal |
| Edge cache | Managed edge network | One response cache in the ingress with rules and purge. See [CDN and caching](/cdn-and-caching). It is a single cache, not multi-region |
| CDN in front | Built in | Levelrail connects to Cloudflare as the DNS provider, with a per-app proxy switch and automatic cache purge on publish, deploy and rollback, so git, build, publish, DNS and purge are managed from one place: `levelrail-cli apps cdn set <app> --proxied on`. See [Cloudflare in front](/cloudflare-in-front). Not yet verified on a live proxied record. Other CDNs can sit in front, and you purge them with their own tools |
| Identity | Netlify Identity | Not offered |
| Blobs | Netlify Blobs | Not offered. Managed Postgres and Redis exist |
| Image CDN | Image transformation | Image resizing is not yet offered. A CDN in front can cache images you serve |
| Visitor analytics | Netlify Analytics | Not offered. Request rate, errors and latency percentiles per app are built in |
| TLS | Automatic | Self-signed until you enable Let's Encrypt. See [Domains and ingress](/domains-and-ingress#tls-whats-actually-shipped-today) |

[Read the Netlify migration guide](/migrating-from-netlify)

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Is there an importer for Netlify?">
    No. Migration is manual. The guide maps each `netlify.toml` key to a Levelrail setting and marks the ones with no equivalent.
  </Accordion>
  <Accordion title="Do my _redirects and _headers files work?">
    Yes, in Netlify syntax, with the exceptions above. A bad line is skipped and reported in the deploy log, and `levelrail-cli static-sites routes lint` checks the files before you deploy.
  </Accordion>
  <Accordion title="Can I serve a site from a global edge network?">
    Levelrail serves static files from the node or nodes you run, through one cache in the ingress. For edge caching, connect Cloudflare: `levelrail-cli apps cdn set <app> --proxied on` turns the proxy on, and a static publish purges its cache. See [Cloudflare in front](/cloudflare-in-front). Not yet verified on a live proxied record.
  </Accordion>
  <Accordion title="What does it cost?">
    The software is free under Apache 2.0. Your cost is the servers you run it on, rather than usage-based plans.
  </Accordion>
</AccordionGroup>

## Related

- [Migrating from Netlify](/migrating-from-netlify)
- [Vercel alternative](/vercel-alternative)
- [Static pages](/pages)
