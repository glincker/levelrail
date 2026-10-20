---
title: "Dokku alternative: a dashboard, deploy history and multi-server"
description: "Levelrail is an open source Dokku alternative that adds a dashboard, deploy history with rollback, and built-in metrics and logs."
"og:url": "https://docs.levelrail.com/dokku-alternative"
---

**When one server and a command line stops being enough**

Dokku is a classic for a single host. Levelrail can give you a `git push` remote on your own server (beta, off by default, see [Git push deploys](/git-push-deploy)). It deploys from a connected git provider (GitHub, GitLab, Bitbucket or Gitea webhooks), from a public repository URL, or from a container image, or in one command with `levelrail-cli up` from your project directory, and adds a dashboard, deploy history with one-click rollback, built-in observability and a way to add more servers.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## What you gain on top of a deploy command

<CardGroup cols={3}>
  <Card title="A dashboard and history">
    See every deploy, its logs and its status in one place, and roll back to a pinned previous image with one click.
  </Card>
  <Card title="Metrics and logs built in">
    Node-local metrics at 15 second resolution and full-text log search, with deploy markers on the charts.
  </Card>
  <Card title="Room to grow">
    Add servers with a one-time join token and a small agent that dials out. Cordon, drain and explicit placement are supported.
  </Card>
</CardGroup>

## Dokku and Levelrail, side by side

Dokku has almost no idle footprint and is hard to beat for simplicity on one server. These are the design differences.

| | Dokku | Levelrail |
| --- | --- | --- |
| How it manages servers | Commands run on the host itself, driven over SSH or `git push` to the server | A control plane with an agent per node that dials out over mTLS |
| Orchestration | A plugin-based scheduler over Docker on one host | Level-triggered reconciler with recorded status conditions |
| Ingress | nginx by default, other proxies via plugins | Caddy embedded in the control plane |
| Runs as | A set of scripts on the host, no separate web service | One Go binary with an embedded dashboard |
| Observability | Add your own stack | Metrics and log search in the core |

[Read the full comparison](/comparison)

## Getting started

<Steps>
  <Step title="Install on a Linux box">
    The install script handles Docker and the systemd service.
  </Step>
  <Step title="Connect a git repo or point at an image">
    Connect GitHub, GitLab, Bitbucket or Gitea and pushes to your repository trigger deploys through webhooks. Without a provider, run `levelrail-cli import <repo-url> --deploy` for a public repository, or `levelrail-cli deploy <name> --image IMAGE` for a container image.
  </Step>
  <Step title="Deploy and roll back">
    Traffic cuts over only after the readiness probe passes, and previous images stay pinned.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Is there an importer for Dokku?">
    No. You deploy the same repositories and recreate environment variables and domains. The importer covers Coolify, Dokploy and CapRover.
  </Accordion>
  <Accordion title="Does Levelrail use buildpacks?">
    Not Heroku buildpacks. Levelrail builds a Dockerfile with BuildKit, and when there is none it detects the stack from files such as `package.json` or `go.mod` and builds it with Railpack. See [Importing apps](/importing-apps) for the detection order.
  </Accordion>
  <Accordion title="Do I need more than one server?">
    No. Single node is the best tested path, and multi-server is there when you need it.
  </Accordion>
  <Accordion title="Is it free?">
    Yes. Apache 2.0, one edition.
  </Accordion>
  <Accordion title="Does Levelrail support SSO, SAML and SCIM?">
    Levelrail has password, passkey and TOTP sign-in, OAuth sign-in with 17 providers and generic OpenID Connect, IAM policies, teams and a hash-chained audit log, all in the one free edition. SAML single sign-on and SCIM provisioning exist but are beta: they sit behind the `saml-scim` feature toggle, are off by default, and are tested against a test identity provider rather than a live Okta or Entra ID tenant. See [SAML and SCIM](/sso-saml). Dokku has no web login of its own: access is by SSH keys, so SSO is a different question there. See its documentation for plugins.
  </Accordion>
</AccordionGroup>

## Choose Dokku if

Choose Dokku if you want the smallest possible setup on one server: one `git push` to your own host deploys the app, there is no long-running control service, no dashboard to log in to, and no separate account on a git provider. Levelrail costs more to learn and to run (a resident service, a dashboard login, a git provider connection for push deploys) and earns that back once you want history, rollback, metrics, or a second server. Levelrail's zero-DNS HTTPS on a `sslip.io` hostname is the closest counter to Dokku's simplicity.

## Add a dashboard to your deploys

Install it beside your current setup and compare.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
