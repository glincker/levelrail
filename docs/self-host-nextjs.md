---
title: "Self-host Next.js on your own server with git push, TLS and rollback"
description: "How to self-host a Next.js app with Levelrail: Dockerfile build, automatic HTTPS, preview environments per pull request and env vars."
"og:url": "https://docs.levelrail.com/self-host-nextjs"
---

**Run Next.js on a server you own, without the glue scripts**

Build from your Dockerfile, get HTTPS, preview environments per pull request, a managed database and one-click rollback, all from a git push.

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

## Self-hosting Next.js, honestly

What changes when you leave a managed platform.

| | Managed platform | Levelrail on your server |
| --- | --- | --- |
| Cost model | Usage and seat based | Your server cost |
| Edge network | Included | Not provided: put a CDN in front |
| Serverless functions | Included | Not provided: long-lived containers |
| Previews and rollback | Included | Included |

## From repo to HTTPS

<Steps>
  <Step title="Add a Dockerfile">
    A standard multi-stage Next.js Dockerfile with output set to standalone keeps the image small.
  </Step>
  <Step title="Connect the repo and set env vars">
    Set build-time variables first, then runtime secrets.
  </Step>
  <Step title="Add your domain">
    Point DNS at the server and Caddy issues and renews the certificate.
  </Step>
  <Step title="Open a pull request">
    A preview environment deploys and is torn down when the pull request closes.
  </Step>
</Steps>

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
