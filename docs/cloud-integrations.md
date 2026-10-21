---
title: "Cloud integrations"
description: What works today with AWS, Google Cloud, Azure, Cloudflare, DigitalOcean and Hetzner (nodes, DNS, backups, registries, events), and what is planned but not built.
"og:url": "https://docs.levelrail.com/cloud-integrations"
---

This page lists, per cloud, what Levelrail does today and what is only planned. If an item is under Planned, nothing for it exists in the product yet. For the per-app tools catalog (Sentry, PostHog and similar), see [Integrations](/integrations).

[ADR 031](https://github.com/glincker/levelrail/blob/main/adr/031-integration-points.md) records how these fit together. The short version: integrations run outside the control plane, through API tokens and events. There is no plugin runtime.

## Summary

| Cloud | Nodes | DNS and certificates | Backups and storage | Registry |
| --- | --- | --- | --- | --- |
| AWS | EC2 | Route53 | S3 | Any registry that takes a username and password |
| Google Cloud | Compute Engine | Cloud DNS provider code only, not connectable yet (see below) | GCS through S3 interoperability (HMAC keys) | Planned (Artifact Registry preset) |
| Azure | VM provisioner in code, not verified | Planned (Azure DNS) | Planned (Blob Storage) | Not applicable |
| Cloudflare | Not applicable | DNS zones, records and DNS-01 | R2 | Not applicable |
| DigitalOcean | Droplets | Not supported | Not verified (Spaces) | Not applicable |
| Hetzner | Cloud servers | Not supported | Not verified (Object Storage) | Not applicable |

## AWS

Works today:

- **EC2 nodes.** Provision a server and enroll it as a node, with a static key, an assumable role, or the control plane's own AWS identity. Needs a default VPC in the target region. Sizes are a short curated `t3.*` list. See [Node provisioning](/node-provisioning#provider-credentials). Status: exercised against test doubles only, not yet against a real AWS account.
- **Route53.** Zones, records, weighted, failover and multivalue routing, health checks, DNS-01 for wildcards, and automatic records when adding a domain. See [DNS zones and records](/dns).
- **S3.** Backup targets and storage destinations for database backups, app volume backups, [log archive](/log-archive), build cache and audit export. See [Backup targets](/backups-and-storage) and [Object storage](/object-storage).
- **Private images.** Registry credentials take a host, username and password, so any registry that accepts those works.

Planned:

- ECR token refresh preset. ECR passwords expire after 12 hours, so a static registry credential stops working until it is updated by hand.
- EC2 user-data install script snippet for installing the control plane on a fresh instance.
- Terraform snippets.

## Google Cloud

Works today:

- **Compute Engine nodes.** Service account JSON key with the Compute Instance Admin role, and a `default` VPC network. No workload identity federation. This path has not been run against a real project, see [what was not tested](/node-provisioning#what-was-not-tested).

- **GCS storage destinations.** The Google Cloud Storage preset on the S3-compatible destination form uses `storage.googleapis.com`, path-style addressing and region `auto`. Create an HMAC key under Cloud Storage, Settings, Interoperability, for a service account that can write to the bucket. Service account JSON keys do not work here. Backup targets already had a GCS preset with the same endpoint. The Test button runs the same probe as every other destination. Status: tested against a local fake S3 server only, never against a real bucket.

In the codebase, not yet usable:

- **Cloud DNS provider code.** Zones, A, AAAA, CNAME and TXT records, and the record operations DNS-01 needs, written against the Cloud DNS REST API using the same service account JSON as Compute Engine. Tested against a local fake of the API only, never against a real project. It is not connected to anything yet: there is no settings page, CLI command or API route to enable it, and ingress does not use it for wildcard certificates, so you cannot use Cloud DNS from the product today.

Planned:

- Connecting Cloud DNS: settings, UI, CLI, automatic records on domain add, and a DNS-01 module for ingress.
- Artifact Registry preset for registry credentials.
- GCE startup script snippet for installing the control plane.
- Terraform snippets.

## Azure

Nothing here has been run against a real Azure account.

Planned:

- Azure DNS as a DNS provider. It can reuse the same provider interface Cloud DNS uses, but it is not built.
- Azure Blob Storage for backups. Blob uses its own API rather than S3, so it needs a separate uploader. It is not built.

## Cloudflare

Works today:

- **DNS.** Zones, records, import and export, delegation checks and DNS-01 for wildcard certificates, with an API token. Cloudflare wins when both it and Route53 are connected. See [DNS zones and records](/dns).
- **Proxy flag.** `proxied` on A, AAAA and CNAME records, and automatic records with `--dns-proxied`. Set the zone's SSL/TLS mode to Full (strict) when it is on.
- **R2.** Backup targets and storage destinations, with the endpoint filled in from the account id.

Not supported: Cloudflare Tunnel, Workers, and Cloudflare [load balancing](/load-balancing) (routing policies are refused on Cloudflare).

## DigitalOcean

Works today:

- **Droplet nodes.** Personal access token with read and write scope. The size picker shows live monthly prices. See [Node provisioning](/node-provisioning).

Not verified:

- **Spaces** is S3 compatible, so the `custom` provider with a Spaces endpoint is expected to work, but it has not been tested and is not listed as supported.
- DigitalOcean DNS is not a supported DNS provider.

## Hetzner

Works today:

- **Cloud server nodes.** Project API token with Read and Write access, live monthly prices in the size picker. See [Node provisioning](/node-provisioning).

Not verified:

- **Object Storage** is S3 compatible, with the same caveat as Spaces above.
- Hetzner DNS is not a supported DNS provider.

## Not tied to one cloud

| Area | What exists |
| --- | --- |
| Git sources | GitHub, GitLab, Bitbucket and Gitea, see [Git integrations](/git-integrations) |
| Notifications | Slack, Discord, Telegram, PagerDuty, email, generic webhook and more, see [Observability](/observability#notification-channels) |
| Data export | Loki, syslog, HTTP, OTLP, Prometheus scrape, audit webhook and audit bucket, see [Data export](/data-export) |
| Automation | Scoped API tokens, the CLI and the MCP server, see [Developers](/developers) and [MCP](/mcp) |
| Templates | Built-in catalog and custom templates, see [Templates and registry](/templates-and-registry) |

## Planned, in one list

None of these exist yet.

- Connecting the Cloud DNS provider (settings, UI, CLI, DNS-01)
- Azure DNS provider
- Azure Blob Storage backups
- Artifact Registry preset
- ECR token refresh preset
- EC2 user-data and GCE startup install scripts
- Terraform snippets
- Community template repository, reviewed by pull request

Not planned until there is demand: AWS Marketplace listing, a plugin runtime, Kubernetes CRDs.
