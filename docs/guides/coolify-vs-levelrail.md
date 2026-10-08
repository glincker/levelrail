---
title: Coolify vs Levelrail for self-hosters
description: "A fair Coolify vs Levelrail comparison for self-hosters: how each manages servers, what each does well, what Levelrail lacks, and how to choose between them."
---

# Coolify vs Levelrail for self-hosters

Coolify and Levelrail both turn your own Linux servers into a place where you push code and get a running app with TLS. They get there differently, and each has real strengths. This guide is for deciding between them. The author works on Levelrail, so statements about Coolify stay inside what its public documentation says, and anything we have not checked is marked.

Levelrail has no stable release yet and is the younger project. This guide compares design, not maturity.

## The short version

| Question | Coolify | Levelrail |
| --- | --- | --- |
| How does it reach a server? | SSH from the control plane | An agent on the node dials out to the control plane over gRPC with mutual TLS |
| What does it run on a server? | Docker and Docker Compose per resource | A level-triggered reconciler drives the Docker Engine API |
| Ingress | Traefik (Caddy is also offered) | Caddy, embedded in the control plane |
| Template catalog | The largest of the self-hosted platforms | A smaller curated catalog |
| Metrics and logs | See Coolify's own docs | Collected per node and queried from the dashboard, no extra install |
| License | See Coolify's own docs | Apache 2.0, one edition, no license key |

## When Coolify is the better choice

- You want the broadest one-click service list today. Coolify has the biggest catalog, and Levelrail does not match it yet.
- You manage servers you already have and do not want to install anything on them besides Docker. SSH-driven management needs no agent.
- You want a larger community and more years of field use. Levelrail has fewer deployments and less accumulated knowledge.

## When Levelrail may fit better

- You do not want inbound SSH from a control plane to every server. Levelrail agents dial out, so a managed node opens no inbound ports and works behind NAT.
- You want a deploy to count as healthy only after the new container passes its readiness probe, with the old release still serving until then. See [zero-downtime deploys](/guides/zero-downtime-deploys-without-kubernetes).
- You want metrics, log search, and alerts without installing a separate stack. See [observability](/observability).
- You want SSO (Google, GitHub, Microsoft, or generic OIDC), IAM policies, and an audit log in the same free edition. See [identity and access](/identity-and-access).

## What Levelrail does not do yet

- No SAML or SCIM. OAuth and OIDC sign-in work.
- A smaller template catalog than Coolify.
- Public ACME certificate issuance has had less field testing than the rest of the ingress.
- No Windows or non-Linux nodes, and no Kubernetes compatibility layer. Those are deliberate non-goals.

The full list is on [feature status](/feature-status) and the [roadmap](/roadmap).

## Footprint

The control plane measured about 68 MB resident at zero apps, about 83 MB at 100 apps and about 91 MB at 500 apps, with idle CPU between 0.03 and 1.6 percent. Read the caveats on [performance](/performance) before quoting this: one run, a development build, every app suspended so no containers were running. No Coolify numbers were measured with the same harness, so this guide makes no speed comparison.

## Moving over from Coolify

The platform importer reads a live Coolify instance, shows a dry run, and creates apps only after you confirm. It does not move database contents, volume contents, certificates, or history. Read [migrating from Coolify, Dokploy and CapRover](/migrating-from-coolify-dokploy-and-caprover) before planning a cutover.

## Next steps

- [Full comparison with Dokploy, CapRover, Dokku and Kamal](/comparison)
- [Coolify alternative](/coolify-alternative)
- [Getting started](/getting-started)
