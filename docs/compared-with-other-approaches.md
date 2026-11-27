---
title: "Compared with other approaches"
description: "Levelrail compared by category with managed platforms, Kubernetes platforms, plain Docker Compose with scripts and DIY tool stacks: who does what."
"og:url": "https://docs.levelrail.com/compared-with-other-approaches"
---

There are four common ways to run applications. This page compares them by category and says when each one fits. Product-by-product pages are linked at the end.

## At a glance

| Task | Managed platform | Kubernetes platform | Docker Compose and scripts | DIY tool stack | Levelrail |
| --- | --- | --- | --- | --- | --- |
| Where apps run | Provider's infrastructure | Cluster you run or rent | Your servers | Your servers | Your servers or cloud accounts |
| Deploy from git | Built in | You add CI and manifests or charts | You write scripts | You wire a CI runner | Built in ([Deploying apps](/deploying-apps)) |
| HTTPS and domains | Built in | You add an ingress and certificate manager | You run a proxy and a certificate client | You wire both | Built in ([Domains and ingress](/domains-and-ingress)) |
| Databases and backups | Add-ons, often priced separately | You run operators or external services | You write dump scripts | You choose and schedule tools | Built in, with restore checks ([Managed databases](/managed-databases)) |
| Logs, metrics, alerts | Built in or add-ons | You deploy a monitoring stack | You ship logs yourself | You run and connect each tool | Built in ([Observability](/observability)) |
| Preview environments | Built in | You build it | You build it | You build it | Built in ([Preview environments](/preview-environments)) |
| Access control and audit | Plan dependent | Cluster roles, audit setup is yours | Server accounts | Per tool | Built in ([Identity and access](/identity-and-access)) |
| AI agent access | Varies | You build it | You build it | You build it | MCP server with permission modes ([AI control](/ai-control)) |
| Pricing model | Usage or seats | Infrastructure plus operator time | Infrastructure | Infrastructure plus operator time | Infrastructure only ([Lower infrastructure cost](/lower-infrastructure-cost)) |

## Managed platforms

You hand the servers to a provider and pay for usage or seats. Setup time is minimal and there is no server to patch. This fits teams that want no operations work and accept the provider's regions, limits and pricing. Levelrail fits when you want apps on your own servers or cloud accounts, with data and logs staying there. You can move in with the [migration guides](/migrate-to-levelrail).

## Kubernetes platforms

Kubernetes gives a large ecosystem, many nodes and fine control over scheduling. It fits big fleets and teams with platform engineers. Levelrail targets smaller fleets (see [Who Levelrail is for](/use-cases)) and works next to Kubernetes: you can [import Kubernetes manifests](/import-kubernetes) into apps, and [connect a cluster](/kubernetes-clusters) you already run (experimental, see [Feature status](/feature-status)).

## Plain Docker Compose and scripts

Compose on one server is simple and transparent. You add the proxy, certificates, backups, deploy automation and monitoring yourself. This fits one or two small apps. Levelrail runs Compose files directly ([Compose apps](/compose-apps)) and exports any app back to plain Compose ([Leave anytime](/leave-anytime)), so starting there costs you nothing later. See [Migrate from Docker Compose](/migrate-from-docker-compose).

## DIY tool stacks

A proxy, a CI runner, a backup tool, a metrics stack, a log store and a secrets manager, each chosen and configured by you. This gives full control over every part and suits teams that want specific tools. The cost is the integration and upgrade work across all of them. Levelrail packages those jobs in one binary and still lets you use outside tools through [Prometheus scrape](/prometheus-scrape), [OTLP ingest](/otlp-ingest) and [event webhooks](/event-webhooks).

## Product pages

For named products, see [Self-hosted PaaS](/self-hosted-paas) and the alternative pages in the Compare and choose group.
