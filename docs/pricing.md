---
title: "Pricing: Levelrail is free and open source (Apache 2.0)"
description: "Levelrail costs nothing: Apache 2.0, one edition, no license key and no feature gated behind a plan. You only pay for your servers."
"og:url": "https://docs.levelrail.com/pricing"
---

**Free. One edition. No feature behind a paywall.**

Levelrail is Apache 2.0 software you run on your own servers. There is no license key, no enterprise tier and no usage meter. Your cost is the servers you already rent.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## Included for everyone

<CardGroup cols={3}>
  <Card title="Sign-in and access control">
    Email and password, TOTP two-factor, OAuth with Google, GitHub or any OpenID Connect provider, and AWS-style Allow and Deny IAM policies scoped to a single app or database.
  </Card>
  <Card title="Audit log and approvals">
    Every sensitive request is recorded with actor and caller surface, queryable and exportable as CSV. Protected environments, deploy approvals and freeze windows are built in.
  </Card>
  <Card title="Backups that get verified">
    Scheduled database and volume backups with retention, restore, and automatic verification after each run, plus snapshots of the control plane itself.
  </Card>
  <Card title="Observability and alerts">
    Node-local metrics, full-text logs, crashloop detection and alerts across 18 notification channel kinds, all in the same binary.
  </Card>
  <Card title="Multi-server">
    Add servers with a one-time join token and a small agent. Cordon, drain and explicit placement are included.
  </Card>
  <Card title="AI-ready API">
    An MCP server exposes the same API and permission model to AI tools, so agents can list apps, read logs and diagnose a crashloop.
  </Card>
</CardGroup>

## What you actually pay for

The only cost is infrastructure: the Linux servers you run Levelrail on and any cloud provider, object storage or registry you choose to connect. A single small VPS is enough to start.

There is no hosted plan today. Levelrail is self-hosted software only, so there is nothing to subscribe to.

## Why it stays this way

Levelrail has one edition by design. Features such as OAuth and OIDC sign-in, SAML SSO (beta, off by default), audit logging and fine-grained roles ship in the same free binary. The comparison page covers how Levelrail differs from other projects and notes that their plans change over time.

## At a glance

- **$0**: software cost
- **1**: edition for everyone
- **0**: features gated by plan
- **Apache 2.0**: license

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Is Levelrail really free?">
    Yes. It is Apache 2.0 licensed with a single edition. There is no license key, no enterprise tier and no feature gated behind a plan.
  </Accordion>
  <Accordion title="Is there paid support?">
    Not today. Help is through GitHub Discussions, issues and the community Discord.
  </Accordion>
  <Accordion title="Can I use it commercially?">
    Yes. The Apache 2.0 license permits commercial use, modification and redistribution, subject to its terms.
  </Accordion>
  <Accordion title="Will features move behind a paywall later?">
    The project's stated position is one edition for everyone. Releases that are already published remain under Apache 2.0.
  </Accordion>
</AccordionGroup>

## Start for free

Install it on one server in a few minutes and see whether it fits.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
