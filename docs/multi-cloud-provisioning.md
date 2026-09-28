---
description: Quick reference for choosing a cloud provider and adding a provisioned node in a handful of commands. Full detail on auth modes, cost and known gaps lives on the node provisioning page this one points to.
---

# Multi-cloud provisioning quickstart

[Node provisioning](/node-provisioning) covers how cloud provisioning works end to end: the join-token flow it drives, the cloud-init script, status polling, and the known gaps per provider. This page is the short version: what the four supported providers are, what credential each one needs, roughly what they cost, and the fastest path to a running node.

## Providers at a glance

| Provider | Auth mode | Setup before the first server |
| --- | --- | --- |
| Hetzner Cloud | project API token | none, a token from the Hetzner console is enough |
| DigitalOcean | personal access token | none |
| Azure | service principal JSON (tenant, client, secret, subscription, resource group) | an existing resource group |
| GCP | service account JSON key | a project with the default VPC network present (every new project has one) |

AWS is not supported yet.

Every provider bills by the hour for however long the server exists, and none of them get deleted automatically when you stop using a node; see [node provisioning](/node-provisioning#cost-expectations) for the current per-provider cost range and the delete path. Hetzner and DigitalOcean are the simplest to start with because a single token is the whole setup; Azure and GCP need the resource group or project prepared first.

## Add a node on Hetzner in 3 commands

```
levelrail-cli nodes providers set-credential --provider hetzner
# paste or pipe the project API token when prompted

levelrail-cli nodes provision --provider hetzner --region fsn1 --size cx22 --name build-1

levelrail-cli nodes provisions show <id>
# repeat until status is "ready"; the id is printed by the provision command above
```

The region and size ids above (`fsn1`, `cx22`) are illustrative Hetzner examples, not guaranteed to be the cheapest or currently available option on your account. Check `GET /api/v1/node-providers/hetzner/regions` and `.../sizes?region=<id>` (or the Nodes page's Add Node wizard, which lists both live) before provisioning for real.

The same three-command shape works for DigitalOcean, Azure and GCP: store the credential once with `nodes providers set-credential --provider <name>`, then `nodes provision --provider <name> --region <id> --size <id> --name <name>`. Azure and GCP additionally need the resource group or service account details folded into the credential JSON itself, described on [node provisioning](/node-provisioning#required-token-scopes).
