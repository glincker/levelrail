---
description: Complete index and guide to Levelrail's documentation organized by task and information type.
---

# Levelrail docs

This directory is the source of truth for Levelrail's user-facing and contributor-facing documentation.

**It ships with the repo, not the binary.** Nothing under `/docs` is embedded into the control plane or Docker image. The same files are published at [levelrail.com](https://levelrail.com) and render on GitHub too.

| Start here | |
| --- | --- |
| [Install in one command](installing.md) | Linux server to dashboard in about ten minutes |
| [Deploy a first app](getting-started.md) | From the dashboard or the CLI |
| [Compare with Coolify and Dokploy](comparison.md) | Sourced, honest, includes where Levelrail trails |
| [Feature status](feature-status.md) and [roadmap](roadmap.md) | What is stable, beta, or planned |
| [Security policy](../SECURITY.md) | Report a vulnerability privately |

**New here?** Read [Getting started](getting-started.md), then [Installing](installing.md) if you want the details behind the one-line install.

## How this index is organized

Docs follow the [Diátaxis](https://diataxis.fr) framework: organize by what the reader is trying to do, not which package the content describes.

Four main types, plus two Levelrail-specific categories:

| Type | Answers | Example |
| --- | --- | --- |
| Tutorial | "Walk me through it" | Getting started |
| How-to guide | "How do I do X" | Rotate the master key |
| Reference | "What are the exact fields/rules" | app.yaml schema |
| Explanation | "Why is it built this way" | Architecture, comparison |
| Design proposal | "Here's a proposed shape, not yet decided" | `design/` |
| Status | "What's actually done vs planned, as of when" | Roadmap |

## Index

### Tutorials

| Doc | Covers |
| --- | --- |
| [getting-started.md](getting-started.md) | Install on a Linux server, sign in, and deploy a first app from the dashboard or the CLI |
| [tutorials/](tutorials/index.md) | Step-by-step walkthroughs: deploy a Docker app, zero-downtime deploys, GitHub Actions, Postgres, S3 backups, self-hosting Vaultwarden, logs and metrics |

### How-to guides

#### Getting Started and Installation

| Doc | Covers |
| --- | --- |
| [installing.md](installing.md) | Pre-flight requirements, every install path (`install.sh`, Docker, source), verifying, upgrading, and uninstalling |
| [docker.md](docker.md) | Run the control plane and node agent as containers instead of `install.sh` |

#### Deploying Apps and Git Sources

| Doc | Covers |
| --- | --- |
| [importing-apps.md](importing-apps.md) | The New app import front door: repo URL, docker run, image, compose or Dockerfile in, deployment plan preview out |
| [deploying-apps.md](deploying-apps.md) | An app's lifecycle: create, deploy, roll back, promote, health checks, resource limits, exec, and scheduled tasks |
| [github-actions.md](github-actions.md) | Deploy from a GitHub Actions workflow with the bundled composite Action |
| [git-integrations.md](git-integrations.md) | Connect GitHub, GitLab, and Bitbucket, webhooks, and preview environments |
| [pipelines.md](pipelines.md) | Test, build, approve, and deploy with YAML pipelines: triggers, matrix, secrets, and approvals |

#### Build and Deployment Options

| Doc | Covers |
| --- | --- |
| [previews.md](previews.md) | Preview environments per pull request: domain and DNS, resources, idle sleep, database strategy, TTL and cap, fork policy, basic auth gate |
| [deploy-previews.md](deploy-previews.md) | Opt-in thumbnails of each deploy, captured by a short-lived browser container: cost, privacy, retention and every `APP_PREVIEW_*` setting |
| [supply-chain.md](supply-chain.md) | SBOM per Dockerfile build, an optional vulnerability scan in a short-lived container and a release gate: cost, coverage, retention and every `APP_BUILD_ATTEST` and `APP_SCAN_*` setting |

#### Domains, TLS, and Ingress

| Doc | Covers |
| --- | --- |
| [domains-and-ingress.md](domains-and-ingress.md) | Why there's no reverse proxy to install, how `app.yaml` domains route to containers, and TLS's current honest status |
| [acme-verification-runbook.md](acme-verification-runbook.md) | Verify real ACME certificate issuance against a live domain, step by step |
| [load-balancing.md](load-balancing.md) | Balance traffic across replicas and nodes with health checks, sticky sessions, weights and graceful cutovers, and export the setup as Terraform, CDK, CloudFormation or Caddy |

#### Databases and Backups

| Doc | Covers |
| --- | --- |
| [managing-databases.md](managing-databases.md) | Create and manage Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse resources |
| [backups-and-storage.md](backups-and-storage.md) | Backup targets, registry credentials, and app volume backups |

#### Observability and Monitoring

| Doc | Covers |
| --- | --- |
| [observability.md](observability.md) | Node-local metrics and log storage, federated queries, and the alert engine |
| [deployments-page.md](deployments-page.md) | The cross-app Deployments page: live feed, filters, details drawer, actions and keyboard shortcuts |
| [cost-estimate.md](cost-estimate.md) | The per-app "what this would cost elsewhere" estimate: the formula, reference providers, and how to correct the rates for your own region |

#### Multi-Node Setup

| Doc | Covers |
| --- | --- |
| [multi-node.md](multi-node.md) | Add and manage additional nodes, node health, and simple spread placement |

#### Organization and Access Control

| Doc | Covers |
| --- | --- |
| [projects-and-organizations.md](projects-and-organizations.md) | The optional organization/project/environment grouping hierarchy for apps and databases |
| [service-topology-graph.md](service-topology-graph.md) | A project's apps, databases, and shared volumes drawn as a diagram, with real derived edges |
| [identity-and-access.md](identity-and-access.md) | Users, roles, abilities, IAM policies, invites, tokens, 2FA, OAuth, and audit logging |
| [tags.md](tags.md) | Label and organize apps with arbitrary tags for filtering and grouping |

#### Security and Advanced Topics

| Doc | Covers |
| --- | --- |
| [master-key-rotation.md](master-key-rotation.md) | Rotate the envelope-encryption master key without losing access to stored secrets |
| [migrating-from-coolify-dokploy-and-caprover.md](migrating-from-coolify-dokploy-and-caprover.md) | Move apps off a live Coolify, Dokploy, or CapRover instance with `levelrail-cli migrate` |
| [feature-flags.md](feature-flags.md) | Toggle app behavior at runtime without a redeploy |
| [platform-as-code.md](platform-as-code.md) | Describe projects, environments, apps, domains and databases as YAML, then export, diff, plan and apply them from the CLI, the dashboard, MCP or CI |
| [templates-and-registry.md](templates-and-registry.md) | Deploy curated service templates from the catalog as Compose-backed apps |
| [ai-assistant.md](ai-assistant.md) | Run `levelrail-mcp` over stdio or the network for an MCP-compatible AI assistant, and scope a token for it |
| [ai-assistant-chat.md](ai-assistant-chat.md) | The in-app dashboard/CLI chat behind the `ai-chat` experimental flag: enabling it, what it can and can't do, and the confirmation gate |
| [agent-tooling-audit.md](agent-tooling-audit.md) | Tool counts and estimated token cost per MCP mode, the heaviest and overlapping tools, and the budget test |

#### Maintenance and Documentation

| Doc | Covers |
| --- | --- |
| [screenshots.md](screenshots.md) | Regenerate the dashboard screenshots used in the README |

### Reference

| Doc | Covers |
| --- | --- |
| [app-spec-reference.md](app-spec-reference.md) | Every `app.yaml` field, validated against `internal/spec`'s JSON Schema |
| [feature-catalog.md](feature-catalog.md) | Every dashboard page, API resource group, and CLI command group, plus known UI gaps |
| [cli-reference.md](cli-reference.md) | Every `levelrail-cli` command, organized by command group, extracted from source |
| [mcp-tool-surface.md](mcp-tool-surface.md) | Estimated model context cost of the MCP tool list per toolset, and the `agent-core` profile |
| [api-reference.md](api-reference.md) | Every REST route grouped by resource, with ability and handler |

### Explanation

| Doc | Covers |
| --- | --- |
| [architecture.md](architecture.md) | How Levelrail is actually built today: reconciler, ingress, builds, storage |
| [resilience.md](resilience.md) | What survives a control plane process crash and what does not, measured live: running containers, node agents, and the embedded ingress outage window |
| [threat-model.md](threat-model.md) | Trust boundaries, assets, attackers, mitigations with file references, known gaps, and how to report a vulnerability |
| [security-alert-verdicts.md](security-alert-verdicts.md) | Verdict and evidence for each code scanning alert: fixed, false positive, or accepted risk |
| [comparison.md](comparison.md) | How Levelrail differs from Coolify, Dokploy, CapRover, Dokku, Kamal |

### Design proposals

Pre-ADR proposals: a real shape under discussion, not yet a locked
decision (see `/adr` for decisions that have been made). Status is
noted per-document since these move between draft, proposed, accepted
(promoted to an ADR), and rejected.

| Doc | Status | Covers |
| --- | --- | --- |
| [design/git-provider-integrations.md](design/git-provider-integrations.md) | Proposed | Shared abstraction across GitHub Enterprise Server, Bitbucket, and the connect-flow UX |

### Status

| Doc | Covers |
| --- | --- |
| [roadmap.md](roadmap.md) | What's Done, In progress, and explicitly out of scope, kept current against `main` |
| [feature-status.md](feature-status.md) | Maturity label and test evidence per feature, and README claims checked against the code |
| [experimental-features.md](experimental-features.md) | The `APP_EXPERIMENTAL` switch, what each gated feature does while off, and how the CLI, MCP, and web read it |
| [ci.md](ci.md) | How the CI lanes, required checks and local hooks fit together |
| [performance.md](performance.md) | Measured idle CPU, memory, and API latency at 0, 100, and 500 apps, and how to reproduce it |

## Support and contributing

- **Bug or question?** Open an issue on [GitHub](https://github.com/glincker/levelrail/issues).
- **Quick question?** Ask in the `#levelrail` forum on the [GLINR Discord](https://discord.gg/Ar5pcaZB99).
- **Everything else?** Email [support@levelrail.com](mailto:support@levelrail.com).
- **Security vulnerability?** Don't open a public issue; see [Security overview](security.md#reporting-a-vulnerability) or the repository's [SECURITY.md](../SECURITY.md).
- **Want to contribute code?** See the repository's [CONTRIBUTING.md](../CONTRIBUTING.md) for branch naming, commit conventions, and how to run tests and the linter before opening a PR.

## Adding a new doc

1. Pick the Diátaxis type first (see the table above), not the package. Mixed reference and tutorial prose is the most common way docs rot, because neither reader gets what they need.

2. Add it to the matching heading in the Index section above.

3. Link it from the root README only if it is something a new user or contributor would hit early. Leave specialized how-tos and reference pages reachable only from here, so the root README stays focused.

## Writing style

- **Write for the operator, not the codebase.** A how-to or tutorial explains what a reader can do and why it matters to them. Package names, file paths, and Go/TS identifiers belong in an Explanation doc (architecture.md and friends), or in a `::: details For contributors: ...` block at the point where a contributor would actually need them, never in the opening paragraph of a page a new user lands on first.
- **Show, don't just tell.** If a real screenshot exists or would help (`docs/assets/screenshots/`, regenerated by `scripts/screenshots/capture.sh`, see [screenshots.md](screenshots.md)), embed it with `![alt text](assets/screenshots/name.png)`. If a flow has more than two or three steps that branch or loop, a `\`\`\`mermaid` diagram usually reads faster than the same steps in prose. Don't add either decoratively: a diagram earns its place only if it actually clarifies something prose alone wouldn't.
- **State what's true, not what's aspirational.** "Not built yet" belongs in a dedicated section (see observability.md's "Not built yet") or in roadmap.md, never blended into the middle of a paragraph describing what exists today.
- **No hedging, no filler.** Skip "simply," "just," "basically," "note that," and sentences that restate the heading above them. If a sentence would be identical with the qualifier removed, remove the qualifier.
- **No em dashes or en dashes anywhere** (repo-wide rule, enforced by the pre-commit hook): use commas, periods, or parentheses instead.
- **Short paragraphs, real headings.** A reader scanning for one answer should be able to find it from the heading list alone. If a section covers more than one question, split it.
