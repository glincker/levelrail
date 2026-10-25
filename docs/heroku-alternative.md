---
title: "Self-hosted Heroku alternative: push-to-deploy on your own servers"
description: "Levelrail is an open source Heroku alternative for your own Linux servers: push-to-deploy from your git host, managed Postgres and Redis, TLS, logs and rollback."
"og:url": "https://docs.levelrail.com/heroku-alternative"
---

**The Heroku workflow on a server you control**

Push-to-deploy from your git host, managed databases, HTTPS (self-signed until you enable Let's Encrypt), logs and rollback, running on Linux boxes you own instead of per-dyno billing. There is no `git push heroku` style remote by default (a native `git push` remote exists but is beta and off, see [Git push deploy](/git-push-deploy)): deploys come from a connected git provider, a repository URL or a container image.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## What carries over from Heroku

<CardGroup cols={3}>
  <Card title="Push to deploy">
    Connect GitHub, GitLab or Bitbucket and every push to a branch builds and deploys with a readiness-gated cutover.
  </Card>
  <Card title="Managed databases">
    Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly and ClickHouse as first-class resources, with scheduled backups and automatic verification.
  </Card>
  <Card title="Logs, metrics and alerts">
    Full-text log search, node-local metrics and alerts to Slack, Discord, email, PagerDuty and more, with no add-on subscriptions.
  </Card>
</CardGroup>

## Heroku and Levelrail

Heroku is a managed platform. Levelrail is the same workflow on infrastructure you operate.

| | Heroku | Levelrail |
| --- | --- | --- |
| Billing | Per dyno and per add-on | Your own servers, no per-app fee |
| Deploys | git push to Heroku, buildpacks, release phase | Push to your git host (webhook), Dockerfile and Railpack builds, readiness-gated cutover |
| Databases | Add-ons | Eight engines managed in the platform, scheduled and verified backups |
| Operations | Handled by the vendor | You operate the servers, with alerts and a status page to help |

[Read the full comparison](/comparison)

## A first deploy

<Steps>
  <Step title="Pick a server">
    Any Linux box with Docker works, or provision one from Hetzner, DigitalOcean, AWS, Azure or GCP.
  </Step>
  <Step title="Install and connect git">
    Run the install script, open the dashboard, and connect a repository.
  </Step>
  <Step title="Add a database and deploy">
    Create Postgres or Redis, link it to the app, and push.
  </Step>
</Steps>

## Moving an existing Heroku app

Two offline or read-only helpers exist, and neither changes anything on Heroku:

- `levelrail-cli import config` reads `Procfile` and `app.json` from a directory and prints a proposed `app.yaml`, with findings for what has no equivalent (each with a fix). It is offline and needs no token.
- `levelrail-cli import platform heroku --dry-run` reads your apps through the Heroku Platform API with a token and reports what would be created.

To move the Postgres data itself, create a managed database and run `levelrail-cli databases import-hosted` while the Heroku app is in maintenance mode. The steps and limits are in [Move a hosted Postgres](/migration-reference#move-a-hosted-postgres-heroku).

The two helpers above are previews you review, not a one-click migration. Add-ons and anything the findings flag as a blocker still need manual work. See [Migrate to Levelrail](/migrate-to-levelrail).

### Emit it as a stack and review it in a pull request

Instead of creating apps straight away, `levelrail-cli import platform heroku --emit stack --out DIR` reads your Heroku apps and add-ons (read only) and writes a Stack with App, Database and Domain documents, a `SECRETS.md` listing config var names (never values) and a `MIGRATION.md` of what has to be done by hand. Commit the directory, read the plan with `levelrail-cli stack plan`, and apply it when the pull request merges. See [Stacks and GitOps](/stacks#migrate-into-a-stack).

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Do I need to learn Docker?">
    Not deeply. If your repo has a Dockerfile, it deploys. Levelrail handles the runtime, TLS and routing.
  </Accordion>
  <Accordion title="What replaces Heroku add-ons?">
    Managed databases, built-in metrics and logs, alerting channels and scheduled backups cover the common ones. Third-party add-ons are not replicated.
  </Accordion>
  <Accordion title="Who is on call?">
    You are, since you run the servers. Alerts, crashloop detection and a status page are built in to make that manageable.
  </Accordion>
  <Accordion title="Is it free?">
    Yes, Apache 2.0, one edition.
  </Accordion>
  <Accordion title="Does Levelrail support SSO, SAML and SCIM?">
    Levelrail has password, passkey and TOTP sign-in, OAuth sign-in with 20 providers and generic OpenID Connect, IAM policies, teams and a hash-chained audit log, all in the one free edition. SAML single sign-on and SCIM provisioning exist but are beta: they sit behind the `saml-scim` feature toggle, are off by default, and are tested against a test identity provider rather than a live Okta or Entra ID tenant. See [SAML and SCIM](/sso-saml).
  </Accordion>
</AccordionGroup>

## Stop paying per dyno

Try it on one small server and move a project across when it feels right.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
