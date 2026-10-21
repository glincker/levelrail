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
| AWS | EC2 | Route53 | S3 | ECR, refreshed automatically |
| Google Cloud | Compute Engine | Cloud DNS provider code only, not connectable yet (see below) | GCS through S3 interoperability (HMAC keys) | Artifact Registry, refreshed automatically |
| Azure | VM provisioner in code, not verified | Planned (Azure DNS) | Planned (Blob Storage) | ACR, refreshed automatically |
| Cloudflare | Not applicable | DNS zones, records and DNS-01 | R2 | Not applicable |
| DigitalOcean | Droplets | Not supported | Not verified (Spaces) | Not applicable |
| Hetzner | Cloud servers | Not supported | Not verified (Object Storage) | Not applicable |

## AWS

Works today:

- **EC2 nodes.** Provision a server and enroll it as a node, with a static key, an assumable role, or the control plane's own AWS identity. Needs a default VPC in the target region. Sizes are a short curated `t3.*` list. See [Node provisioning](/node-provisioning#provider-credentials). Status: exercised against test doubles only, not yet against a real AWS account.
- **Route53.** Zones, records, weighted, failover and multivalue routing, health checks, DNS-01 for wildcards, and automatic records when adding a domain. See [DNS zones and records](/dns).
- **S3.** Backup targets and storage destinations for database backups, app volume backups, [log archive](/log-archive), build cache and audit export. See [Backup targets](/backups-and-storage) and [Object storage](/object-storage).
- **ECR.** The ECR preset calls GetAuthorizationToken with the same AWS connection (static key, assumable role or the control plane identity) and renews the 12 hour password before it expires. Status: exercised against test doubles only, not yet against a real AWS account. See [Cloud registry presets](#cloud-registry-presets).
- **Private images.** Other registries still take a host, username and password.
- **EC2 user-data snippet.** A bash script for installing the control plane on a fresh instance, with a Terraform example. See [Install on a cloud VM](/install-on-cloud). Documentation only, not run against a real account.

Planned:

- Terraform modules. The example on the install page is not a module.

## Google Cloud

Works today:

- **Compute Engine nodes.** Service account JSON key with the Compute Instance Admin role, and a `default` VPC network. No workload identity federation. This path has not been run against a real project, see [what was not tested](/node-provisioning#what-was-not-tested).
- **Startup script snippet.** A bash startup script for installing the control plane, see [Install on a cloud VM](/install-on-cloud). Documentation only.

- **GCS storage destinations.** The Google Cloud Storage preset on the S3-compatible destination form uses `storage.googleapis.com`, path-style addressing and region `auto`. Create an HMAC key under Cloud Storage, Settings, Interoperability, for a service account that can write to the bucket. Service account JSON keys do not work here. Backup targets already had a GCS preset with the same endpoint. The Test button runs the same probe as every other destination. Status: tested against a local fake S3 server only, never against a real bucket.
- **Artifact Registry.** The preset turns the Compute Engine service account JSON into a short-lived access token (username `oauth2accesstoken`) and renews it. Status: exercised against test doubles only, not yet against a real project.

In the codebase, not yet usable:

- **Cloud DNS provider code.** Zones, A, AAAA, CNAME and TXT records, and the record operations DNS-01 needs, written against the Cloud DNS REST API using the same service account JSON as Compute Engine. Tested against a local fake of the API only, never against a real project. It is not connected to anything yet: there is no settings page, CLI command or API route to enable it, and ingress does not use it for wildcard certificates, so you cannot use Cloud DNS from the product today.

Planned:

- Connecting Cloud DNS: settings, UI, CLI, automatic records on domain add, and a DNS-01 module for ingress.
- Terraform modules.

A GCE startup script for installing the control plane is on [Install on a cloud VM](/install-on-cloud) (documentation only).

## Azure

Works today:

- **Virtual machine nodes.** Service principal with a client secret or workload identity federation, scoped to one resource group. See [Node provisioning](/node-provisioning#provider-credentials).
- **ACR.** The ACR preset exchanges an Azure AD token from the same service principal for an ACR refresh token and renews it. You enter the registry host, for example `myregistry.azurecr.io`. Status: exercised against test doubles only, not yet against a real Azure account.
- **Custom data snippet.** A cloud-init file for installing the control plane on a fresh VM, see [Install on a cloud VM](/install-on-cloud). Documentation only.

Planned:

- Azure DNS as a DNS provider. It can reuse the same provider interface Cloud DNS uses, but it is not built.
- Azure Blob Storage for backups. Blob uses its own API rather than S3, so it needs a separate uploader. It is not built.

## Cloud registry presets

Connect ECR, Artifact Registry or ACR from Settings, Registry credentials, or with the CLI. Each preset needs only the matching cloud connection that node provisioning already uses, plus a region or registry host. Test mints a login without saving anything.

```
levelrail-cli registry-credentials create --name prod-ecr --preset ecr --region eu-west-1
levelrail-cli registry-credentials create --name gar --preset artifact_registry --region europe
levelrail-cli registry-credentials create --name acr --preset acr --registry-host myregistry.azurecr.io
levelrail-cli registry-credentials refresh <id>
```

A background refresher renews each credential before it expires and shows the expiry, last refresh and next refresh. The password is stored with the usual envelope encryption and is never returned by the API, logged or written to audit detail. When a refresh fails, the "Registry credential refresh failing" alert rule raises an Inbox item, and the Refresh now button (or `registry-credentials refresh`) retries it. Timings, all Go durations:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_REGISTRY_REFRESH_LEAD` | `30m` | How long before expiry to renew. A login shorter than this renews at half its remaining life. |
| `APP_REGISTRY_REFRESH_INTERVAL` | `1m` | How often the refresher looks for due credentials. |
| `APP_REGISTRY_REFRESH_RETRY` | `5m` | Wait before retrying a failed refresh. |
| `APP_REGISTRY_ACR_FALLBACK_TTL` | `3h` | Assumed lifetime if an ACR token carries no expiry. |

Nothing here has run against a real AWS, Google Cloud or Azure account. The token exchanges are covered by tests against fake endpoints.

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
- Terraform modules
- Community template repository, reviewed by pull request

Not planned until there is demand: AWS Marketplace listing, a plugin runtime, Kubernetes CRDs.
