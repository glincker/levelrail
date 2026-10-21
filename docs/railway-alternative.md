---
title: "Self-hosted Railway alternative with built-in observability"
description: "Levelrail is an open source Railway alternative you run on your own servers: push-to-deploy from your git host, managed databases, domains with TLS."
"og:url": "https://docs.levelrail.com/railway-alternative"
---

**Railway-style simplicity on your own infrastructure**

Connect a repo, get a running service with a domain, TLS, a database and logs. The difference is that it runs on servers you own and the usage meter is yours.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

<img src="/assets/screenshots/apps-list-light.png" alt="Levelrail apps list across nodes" className="block dark:hidden" />
<img src="/assets/screenshots/apps-list.png" alt="Levelrail apps list across nodes" className="hidden dark:block" />

## What you get

<CardGroup cols={3}>
  <Card title="Services, domains and TLS">
    Add a domain and Caddy serves it with an internal (self-signed) certificate by default. Enable Let's Encrypt for trusted certificates; renewal is unverified in a live run. Routing is handled in the control plane, with no separate proxy to run.
  </Card>
  <Card title="Databases next to your app">
    Create a managed database, link it to a service, and connect over the internal network. Backups are scheduled and checked.
  </Card>
  <Card title="Observability without an add-on">
    Metrics, log search and deploy markers overlaid on charts answer which deploy changed things without installing Grafana.
  </Card>
</CardGroup>

## Railway and Levelrail

Railway is a managed cloud. Levelrail is software you run.

| | Railway | Levelrail |
| --- | --- | --- |
| Where it runs | Vendor-managed cloud | Your Linux servers, one to many |
| Pricing model | Usage-based | Your own server cost, nothing per service |
| Previews | Environments per PR | Preview environments per pull request with automatic teardown (stable on GitHub, beta on GitLab and Bitbucket) |
| Data location | With the vendor | On your nodes: secrets, metrics and logs are stored locally unless you configure an export |

[Read the full comparison](/comparison)

## Running in three steps

<Steps>
  <Step title="Install">
    One script on a Linux server.
  </Step>
  <Step title="Connect your repo">
    GitHub, GitLab or Bitbucket.
  </Step>
  <Step title="Add a domain">
    Point DNS, then enable public ACME for a trusted certificate (the default is self-signed).
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Can I scale across several servers?">
    Yes. Extra servers join with a one-time token and a small agent. Placement is explicit, and cordon and drain are supported.
  </Accordion>
  <Accordion title="Is there a managed cloud version?">
    No. Levelrail is self-hosted software only today.
  </Accordion>
  <Accordion title="Can I import from Railway?">
    There is no Railway importer. You deploy from the same git repos and recreate env vars and domains.
  </Accordion>
  <Accordion title="Is it open source?">
    Yes, Apache 2.0.
  </Accordion>
  <Accordion title="Does Levelrail support SSO, SAML and SCIM?">
    Levelrail has password, passkey and TOTP sign-in, OAuth sign-in with 20 providers and generic OpenID Connect, IAM policies, teams and a hash-chained audit log, all in the one free edition. SAML single sign-on and SCIM provisioning exist but are beta: they sit behind the `saml-scim` feature toggle, are off by default, and are tested against a test identity provider rather than a live Okta or Entra ID tenant. See [SAML and SCIM](/sso-saml).
  </Accordion>
</AccordionGroup>

## Run your own Railway

Start with one small server and see how little it asks of it.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
