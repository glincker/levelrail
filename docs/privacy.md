---
title: "Privacy and data handling: what Levelrail stores and sends"
description: "Where your data lives with Levelrail: secrets, metrics and logs stay on your own servers and the product sends no usage telemetry."
"og:url": "https://docs.levelrail.com/privacy"
---

**Your data stays on your servers**

Levelrail is self-hosted. Secrets, metrics and logs live on infrastructure you control, and the platform does not report usage to us.

[Read the security overview](/security) | [Review the source](https://github.com/glincker/levelrail)

## The short version

<CardGroup cols={3}>
  <Card title="Secrets are encrypted per app">
    Each app has its own data encryption key wrapped by a master key held only by the control plane. Agents receive decrypted environment values when a container is created and do not persist them.
  </Card>
  <Card title="Metrics and logs stay node-local">
    Each node keeps its own time series and log store. The control plane queries nodes on demand instead of shipping telemetry to a central service or a third party.
  </Card>
  <Card title="No usage telemetry">
    A search of the Go source for analytics, usage reporting and crash reporting libraries finds none. The code is open, so you can check it yourself.
  </Card>
</CardGroup>

## Outbound connections

Levelrail's own code contains no hardcoded calls to servers operated by us. The one built-in lookup is a release check against GitHub's releases API. The periodic check only runs when an operator opts in to automatic update checks, and the Updates page also queries it when you open it. It records the result and never applies an update by itself.

Everything else that leaves your network is something you configure: git providers for webhooks and clones, an ACME certificate authority for TLS, container registries, cloud provider APIs for node provisioning, object storage for backups, browser push services for devices that subscribe to web push, and the alert channels you set up.

## Agents and nodes

Each node agent dials out to the control plane over mutual TLS. Managed servers need no inbound port, and the agent talks to the local Docker Engine API rather than shelling out.

## AI features

AI is a read and suggest layer on top of the API and is never in the reconciliation path. Where AI features are used, they call the model provider you configure.

## This website

The documentation site's own code loads no analytics scripts and sets no cookies, and its fonts are served from the same origin. Like any web host, whoever serves the files can see ordinary server logs.

This page describes how the software behaves, based on the source at the time of writing, and is not a legal contract. For a vulnerability report, follow the project's security policy on GitHub.

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Does Levelrail send telemetry to GLINCKER?">
    No. The software has no usage reporting. Its metrics collector is node-local and exists to power your own dashboards and alerts.
  </Accordion>
  <Accordion title="Does it phone home?">
    The only built-in lookup is the release check against GitHub's releases API, which runs periodically only if you opt in and when you open the Updates page. Nothing else in the source calls a host operated by the project.
  </Accordion>
  <Accordion title="Where are my secrets stored?">
    In the control plane database, encrypted with per-app keys wrapped by a master key. Agents receive them at container creation time and do not persist them.
  </Accordion>
  <Accordion title="How do I report a security issue?">
    Follow the security policy in the GitHub repository rather than opening a public issue.
  </Accordion>
</AccordionGroup>

## Verify it yourself

The whole platform is Apache 2.0 and public. Read the code, run it, and trace what it connects to.

[Read the security overview](/security) | [Review the source](https://github.com/glincker/levelrail)
