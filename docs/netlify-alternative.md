---
title: "Self-hosted Netlify alternative for static sites and Docker apps"
description: "Levelrail is an open source, self-hosted alternative to Netlify's Git deploys for static sites: build and publish from git, _redirects and _headers, branch deploys, forms and build hooks on your own server."
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

Stay on Netlify if you need a global edge network, Edge Functions, Netlify Identity or built-in analytics. Levelrail has none of them. The gaps are listed in the table below and in the runbook.

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
| Serverless functions | Netlify Functions | [Functions](/functions): container apps that sleep when idle. Not an edge runtime |
| Edge Functions | Supported | Not offered, and a stated non-goal |
| Global CDN | Managed edge network | Not provided. Put your own CDN in front if you need one |
| Identity | Netlify Identity | Not offered |
| Blobs | Netlify Blobs | Not offered. Managed Postgres and Redis exist |
| Image CDN | Image transformation | Not offered |
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
    Not from Levelrail. Static files are served from the node or nodes you run. If you need edge caching, put a CDN in front.
  </Accordion>
  <Accordion title="What does it cost?">
    The software is free under Apache 2.0. Your cost is the servers you run it on.
  </Accordion>
</AccordionGroup>

## Related

- [Migrating from Netlify](/migrating-from-netlify)
- [Vercel alternative](/vercel-alternative)
- [Static pages](/pages)
