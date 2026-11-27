---
title: "Levelrail vs other platforms: Coolify, Dokploy, Vercel, Heroku, Kubernetes and more"
description: "Levelrail compared with Coolify, Dokploy, CapRover, Dokku, Kamal, Heroku, Railway, Vercel, Render, Kubernetes and DIY stacks, and when each fits better."
"og:url": "https://docs.levelrail.com/compared-with-other-approaches"
---

There are four common ways to run applications. This page compares them by category, names the products in each, and says when each one fits. Each product links to its own documentation. Descriptions are general and dated 2026-11-26. Their features and pricing change, so confirm on their sites.

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

Examples: [Heroku](https://devcenter.heroku.com/), [Railway](https://docs.railway.com/), [Vercel](https://vercel.com/docs), [Netlify](https://docs.netlify.com/), [Render](https://render.com/docs) and [Google Cloud Run](https://cloud.google.com/run/docs).

You hand the servers to a provider, who runs them for you. Setup time is minimal and there is no server to patch. This fits teams that want no operations work and accept the provider's regions, limits and pricing, which are listed on each provider's own pricing page. Levelrail fits when you want apps on your own servers or cloud accounts, with data and logs staying there.

Pages for each: [Heroku alternative](/heroku-alternative), [Railway alternative](/railway-alternative), [Vercel alternative](/vercel-alternative), [Netlify alternative](/netlify-alternative), [Cloud Run alternative](/cloud-run-alternative). Moving in: [Migrate from Vercel](/migrate-from-vercel), [Migrate from Netlify](/migrating-from-netlify), [Migrate from Render](/migrating-from-render), [Migrate from Cloud Run](/migrating-from-cloud-run).

## Self-hosted deploy platforms

Examples: [Coolify](https://coolify.io/docs), [Dokploy](https://docs.dokploy.com/), [CapRover](https://caprover.com/docs/), [Dokku](https://dokku.com/docs/getting-started/installation/) and [Kamal](https://kamal-deploy.org/).

These also run on your own servers and cover the deploy workflow. They differ in scope, so check their docs for what each includes. If one of them already covers what you need, it is a reasonable choice and moving costs you time. Levelrail adds a single control plane with built-in observability, preview environments, an AI control plane over MCP, and Terraform, API and CLI parity.

Pages for each: [Coolify alternative](/coolify-alternative), [Dokploy alternative](/dokploy-alternative), [CapRover alternative](/caprover-alternative), [Dokku alternative](/dokku-alternative), [Kamal alternative](/kamal-alternative). A side-by-side with the features checked in this repo is in [Coolify vs Levelrail](/guides/coolify-vs-levelrail) and [vs Coolify, Dokploy, CapRover, Dokku and Kamal](/comparison). Moving in: [Migrate from Coolify](/migrate-from-coolify), [Migrate from Dokploy](/migrate-from-dokploy), [Migrate from CapRover](/migrate-from-caprover), [Migrate from Dokku](/migrate-from-dokku).

## Kubernetes platforms

Examples: [Kubernetes](https://kubernetes.io/docs/home/) itself, lightweight distributions such as [K3s](https://docs.k3s.io/), and platforms built on Kubernetes such as [Rancher](https://ranchermanager.docs.rancher.com/) and [OpenShift](https://docs.redhat.com/en/documentation/openshift_container_platform).

Kubernetes gives a large ecosystem, many nodes and fine control over scheduling. It fits big fleets and teams with platform engineers, and if you already run a cluster for the rest of your company, keeping apps there is a sound choice. Levelrail targets smaller fleets (see [Who Levelrail is for](/use-cases)) and works next to Kubernetes: you can [import Kubernetes manifests](/import-kubernetes) into apps, and [connect a cluster](/kubernetes-clusters) you already run (experimental, see [Feature status](/feature-status)).

## Plain Docker Compose and scripts

Example: [Docker Compose](https://docs.docker.com/compose/) with a shell script or a CI job.

Compose on one server is simple and transparent. You add the proxy, certificates, backups, deploy automation and monitoring yourself. This fits one or two small apps. Levelrail runs Compose files directly ([Compose apps](/compose-apps)) and exports any app back to plain Compose ([Leave anytime](/leave-anytime)), so starting there costs you nothing later. See [Migrate from Docker Compose](/migrate-from-docker-compose).

## DIY tool stacks

Examples: [Traefik](https://doc.traefik.io/traefik/) or [Caddy](https://caddyserver.com/docs/) for the proxy, [Prometheus](https://prometheus.io/docs/) and [Grafana](https://grafana.com/docs/) for metrics, [Loki](https://grafana.com/docs/loki/latest/) for logs, [restic](https://restic.readthedocs.io/) for backups, plus a CI runner.

Each tool focuses on one job, and this gives full control over every part. It suits teams that want specific tools. The cost is the integration and upgrade work across all of them. Levelrail packages those jobs in one binary and still lets you use outside tools through [Prometheus scrape](/prometheus-scrape), [OTLP ingest](/otlp-ingest), the [Loki-compatible API](/loki-api) and [event webhooks](/event-webhooks).

## More

See [Self-hosted PaaS](/self-hosted-paas) for what to look for when choosing.
