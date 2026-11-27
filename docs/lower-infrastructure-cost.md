---
title: "Lower infrastructure cost"
description: "Where Levelrail lowers infrastructure cost: one control plane instead of several paid services, commodity servers, sleep when idle, and no per-seat pricing."
"og:url": "https://docs.levelrail.com/lower-infrastructure-cost"
---

Levelrail is Apache 2.0 software with one edition. There is no license key, no usage meter and no per-seat price ([Pricing](/pricing)). Your cost is the servers and cloud services you choose to connect. This page lists where that cost drops, and links each point to the feature that does it.

## 1. One control plane replaces several services

A typical self-run setup pays for, or operates, separate tools for each job. Levelrail ships these in the same binary:

| Job | Where it lives in Levelrail |
| --- | --- |
| Build and deploy from git | [Deploying apps](/deploying-apps) |
| Reverse proxy and TLS certificates | [Domains and ingress](/domains-and-ingress) |
| Database management and backups | [Managed databases](/managed-databases), [Backups and storage](/backups-and-storage) |
| Metrics, logs and alerts | [Observability](/observability), [Monitors](/monitors) |
| Status page | [Status page](/status-page) |
| Preview environments | [Preview environments](/preview-environments) |
| Access control and audit log | [Identity and access](/identity-and-access), [Audit log](/audit-log) |

Observability data is stored on your own nodes, so there is no separate metrics or logging service to pay for or host. Each replaced service is one less subscription or one less server to keep running.

## 2. Commodity servers and your existing cloud accounts

Levelrail runs on ordinary Linux servers. The control plane idles at about 200 MB of RAM and 1.5 percent of one CPU on a 2 vCPU box, measured with no apps under load ([Benchmarks](/benchmarks), [Performance](/performance)). Your apps use their own resources on top, so a small VPS can host the platform and its apps.

You can also use servers in cloud accounts you already hold. [Node provisioning](/node-provisioning) and [Cloud integrations](/cloud-integrations) create and join servers there, and you pay the provider directly at its normal rates. Levelrail adds no margin.

## 3. Density and sleep when idle

- [Sleep when idle](/sleep-when-idle) stops an app that has had no requests for the time you set. A sleeping app uses no memory or CPU, and the next request starts it. It is per app and off by default. The first request after a sleep waits for the app to boot.
- [Resource-aware placement](/resource-aware-placement) puts new apps and databases on nodes with room for them, and shows why.
- The [resource advisor](/resource-advisor) suggests memory and CPU limits and flags idle apps, so you can right-size servers. Both pages carry a beta label, see [Feature status](/feature-status).

## 4. See and cap what you spend

- [Usage and cost](/usage-and-cost) splits each server's monthly bill across apps by measured CPU and memory use, per app, project, environment and team, with budgets and alerts. It is showback, not an invoice.
- [Spend caps](/spend-caps) set a monthly egress and cost limit per app.
- [Cost estimate](/cost-estimate) shows what an app's measured resources would cost under three reference price profiles.

## 5. No per-seat pricing

Teams, roles, SSO options, the audit log and the API are in the same free edition. Adding a person or an agent token does not change a bill.

## Worked example

This example uses the reference rates from [Cost estimate](/cost-estimate), which are round numbers and not any provider's rate card. An app that uses 1 vCPU and 2 GiB of memory costs, by the formula there:

| Profile | CPU | Memory | Total per month |
| --- | --- | --- | --- |
| `vm` (bare VM) | 1 x 4 = 4 | 2 x 4 = 8 | 12 |
| `paas` (metered container pricing) | 1 x 25 = 25 | 2 x 10 = 20 | 45 |

Use your own server bill and the [Usage and cost](/usage-and-cost) page for real figures.
