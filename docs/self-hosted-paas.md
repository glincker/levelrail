---
title: "Self-hosted PaaS guide: what to look for and how Levelrail fits"
description: "A practical guide to choosing a self-hosted PaaS: how it manages servers, deploys, rolls back, observes, secures and costs."
"og:url": "https://docs.levelrail.com/self-hosted-paas"
---

**What to look for in a self-hosted PaaS**

Running your own Heroku-style platform is a good trade when the platform stays out of the way. These are the questions that decide whether it does, and how Levelrail answers them.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## The six questions

<CardGroup cols={3}>
  <Card title="How does it reach my servers?">
    SSH and CLI shelling work, but they poll and parse text. Levelrail's agent dials out over mutual TLS and uses the Docker Engine API, with no inbound port.
  </Card>
  <Card title="What happens when a deploy fails?">
    Traffic should cut over only after a real readiness probe passes, and the previous release should stay available. Levelrail pins prior images so cleanup cannot remove a rollback target.
  </Card>
  <Card title="Can I see what is happening?">
    Metrics, logs and alerts should not need a second stack. Levelrail keeps node-local metrics and log search in the core.
  </Card>
  <Card title="Where do secrets and data live?">
    Per-app keys wrapped by a master key held only by the control plane, and metrics and logs stay on your nodes.
  </Card>
  <Card title="What is the real cost?">
    Your servers plus your time. Levelrail is Apache 2.0 with one edition and no paywalled features.
  </Card>
  <Card title="How do I leave?">
    Importers and plain Docker underneath keep exit costs low. Levelrail can import from Coolify, Dokploy and CapRover read-only.
  </Card>
</CardGroup>

## Where Levelrail sits

Positioning, not a ranking. Every project below is useful, and the full comparison page is candid about where each is strong.

| | Typical SSH-driven platform | Levelrail |
| --- | --- | --- |
| Server management | SSH and docker CLI commands | Agent over mTLS, Docker Engine API |
| State | Imperative scripts and polling | Desired state converged by a reconciler |
| Observability | Install Grafana or similar | Metrics and logs built in |
| Footprint | A web app plus its own database | One Go binary with embedded SQLite |

[Read the full comparison](/comparison)

## Frequently asked questions

<AccordionGroup>
  <Accordion title="What is a self-hosted PaaS?">
    Software you run on your own servers that gives you a platform-as-a-service workflow: push code, get a running app with TLS, logs and rollback.
  </Accordion>
  <Accordion title="Who is Levelrail for?">
    Operators running roughly 3 to 50 services on 1 to 10 Linux machines who do not want to run Kubernetes.
  </Accordion>
  <Accordion title="Is it a Kubernetes alternative?">
    No. There is no bin-packing scheduler or CRD ecosystem. Replica [autoscaling](/autoscaling) on CPU, memory or request load is built in (beta), and the reconciler pattern runs without the Kubernetes runtime.
  </Accordion>
  <Accordion title="How mature is it?">
    Pre-release and moving fast. The case studies page lists what has been measured and what has not.
  </Accordion>
</AccordionGroup>

## Evaluate it on one server

Install, import read-only, and judge it on your own workload.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
