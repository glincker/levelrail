---
title: "Kamal alternative with a dashboard, history and built-in observability"
description: "Levelrail is an open source Kamal alternative: the same zero-downtime idea plus a control plane, dashboard, deploy history and built-in metrics."
"og:url": "https://docs.levelrail.com/kamal-alternative"
---

**Zero-downtime deploys, plus a control plane you can see**

Kamal is deliberately minimal: no server component, just SSH from your machine or CI. Levelrail keeps readiness-gated cutover and adds a dashboard, history, rollback and observability.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## What a control plane adds

<CardGroup cols={3}>
  <Card title="State you can inspect">
    Desired state lives in a database and a reconciler records a status condition with a reason after every pass, so you can see why something is not ready.
  </Card>
  <Card title="History and rollback">
    Every deploy is recorded with its pinned image. Rolling back is switching to an image that is still there.
  </Card>
  <Card title="No SSH from CI">
    Servers run an agent that dials out to the control plane, so CI talks to the API with a scoped token instead of holding SSH keys.
  </Card>
</CardGroup>

## Kamal and Levelrail, side by side

Kamal is a sharp tool for people who want the fewest moving parts. These are the design differences.

| | Kamal | Levelrail |
| --- | --- | --- |
| How it manages servers | SSH from your machine or CI to each server | Agent per node that dials out over mTLS |
| Orchestration | None: it runs Docker commands per deploy | Level-triggered reconciler |
| Ingress | kamal-proxy | Caddy embedded in the control plane |
| Runs as | A command line tool, no server component | A control plane, an agent per extra node, a dashboard and a CLI |
| Observability | Bring your own | Built in, node-local |

[Read the full comparison](/comparison)

## From a deploy script to a platform

<Steps>
  <Step title="Install the control plane">
    One script on a Linux server.
  </Step>
  <Step title="Deploy an image">
    Use the CLI or the dashboard and watch the readiness gate.
  </Step>
  <Step title="Wire CI to the API">
    Use a scoped API token from GitHub Actions or any pipeline.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Is Levelrail heavier than Kamal?">
    Yes, by design: it is a control plane with a database and a dashboard, while Kamal has no server component. The control plane idles at a small footprint, which the case studies page measures.
  </Accordion>
  <Accordion title="Can I deploy from CI without SSH?">
    Yes. CI calls the API or CLI with a scoped token. See the GitHub Actions guide.
  </Accordion>
  <Accordion title="Is there an importer for Kamal?">
    No. You point Levelrail at the same images and recreate configuration.
  </Accordion>
  <Accordion title="Is it free?">
    Yes. Apache 2.0, one edition.
  </Accordion>
</AccordionGroup>

## Keep the zero-downtime idea, add the dashboard

Try it on one server beside your current setup.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
