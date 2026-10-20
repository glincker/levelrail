---
title: "Self-hosted Vercel alternative for Next.js and Docker apps"
description: "Levelrail is an open source, self-hosted Vercel alternative: push-to-deploy from your git host, automatic TLS and preview environments per pull request."
"og:url": "https://docs.levelrail.com/vercel-alternative"
---

**Vercel-style deploys on servers you own**

Push to git and get a running app with HTTPS, preview environments per pull request, logs, metrics and one-click rollback, on your own Linux boxes.

[Get started](/getting-started) | [Migration guide](/migrating-from-vercel)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## The parts of Vercel people miss when they leave

<CardGroup cols={3}>
  <Card title="Preview environments per PR">
    GitHub, GitLab and Bitbucket webhooks create a preview for each pull request and tear it down automatically when it closes.
  </Card>
  <Card title="One-click rollback">
    Prior images stay pinned, so going back means switching to an image that is still there, not rebuilding from an old commit.
  </Card>
  <Card title="Predictable cost">
    You pay for the servers you already rent, not per request or per seat. Metrics and logs stay on your own nodes.
  </Card>
</CardGroup>

## Where the trade-offs are

Vercel is a managed cloud with a global edge network. Levelrail is the other model: your servers, your data, your bill.

| | Vercel | Levelrail |
| --- | --- | --- |
| Where it runs | A managed cloud you do not control | Linux servers you own, one or many |
| Deploys | Git integration, previews, instant rollback | Git integration, previews per PR, rollback with pinned images |
| Observability | See Vercel's own documentation | Metrics and log search built in, node-local |
| Data | Handled by the vendor | Secrets, metrics and logs stay on your nodes unless you configure an export or drain |
| Edge network | Global edge network | Not provided: bring your own CDN if you need one |

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
    No. It runs long-lived containers on your servers. If you need a global edge network, put a CDN in front.
  </Accordion>
  <Accordion title="Is there an importer for Vercel?">
    No, migration from Vercel is manual. The runbook maps each Vercel concept to its Levelrail equivalent and lists the gaps.
  </Accordion>
  <Accordion title="What does it cost?">
    The software is free under Apache 2.0. Your cost is the servers you run it on.
  </Accordion>
  <Accordion title="Does Levelrail support SSO, SAML and SCIM?">
    Levelrail has password, passkey and TOTP sign-in, OAuth sign-in with 17 providers and generic OpenID Connect, IAM policies, teams and a hash-chained audit log, all in the one free edition. SAML single sign-on and SCIM provisioning exist but are beta: they sit behind the `saml-scim` feature toggle, are off by default, and are tested against a test identity provider rather than a live Okta or Entra ID tenant. See [SAML and SCIM](/sso-saml).
  </Accordion>
</AccordionGroup>

## Related

- [Migrating from Vercel](/migrating-from-vercel)
- [Cloud Run alternative](/cloud-run-alternative)

## Own your deploy pipeline

Run a side project on it first. Open source, so you can read how every step works.

[Get started](/getting-started) | [Migration guide](/migrating-from-vercel)
