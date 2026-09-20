---
description: Complete index and guide to Levelrail's documentation organized by task and information type.
---

# Levelrail docs

This directory is the source of truth for Levelrail's user-facing and contributor-facing documentation.

**It ships with the repo, not the binary.** Nothing under `/docs` is embedded into the control plane or Docker image. It lives on GitHub today; if it moves to a hosted site later, that is a publishing step on top of these files, not a rewrite of them.

**It is plain Markdown, deliberately.** No MDX, no build-tool-specific syntax, no platform-specific frontmatter. Markdown renders anywhere (GitHub, static site generators, README previews, raw repo reads) without conversion. Platform-specific fields (`sidebar_position`, `layout`) get added later if needed, not guessed at now.

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
| [getting-started.md](getting-started.md) | Start self-hosted with `install.sh` or build from source, then deploy a first app |

### How-to guides

| Doc | Covers |
| --- | --- |
| [installing.md](installing.md) | Pre-flight requirements, every install path (`install.sh`, Docker, source), verifying, upgrading, and uninstalling |
| [docker.md](docker.md) | Run the control plane and node agent as containers instead of `install.sh` |
| [feature-flags.md](feature-flags.md) | Toggle app behavior at runtime without a redeploy |
| [screenshots.md](screenshots.md) | Regenerate the dashboard screenshots used in the README |
| [master-key-rotation.md](master-key-rotation.md) | Rotate the envelope-encryption master key without losing access to stored secrets |
| [migrating-from-coolify-dokploy-and-caprover.md](migrating-from-coolify-dokploy-and-caprover.md) | Move apps off a live Coolify, Dokploy, or CapRover instance with `levelrail-cli migrate` |
| [github-actions.md](github-actions.md) | Deploy from a GitHub Actions workflow with the bundled composite Action |
| [domains-and-ingress.md](domains-and-ingress.md) | Why there's no reverse proxy to install, how `app.yaml` domains route to containers, and TLS's current honest status |
| [acme-verification-runbook.md](acme-verification-runbook.md) | Verify real ACME certificate issuance against a live domain, step by step |
| [deploying-apps.md](deploying-apps.md) | An app's lifecycle: create, deploy, roll back, promote, health checks, resource limits, exec, and scheduled tasks |
| [managing-databases.md](managing-databases.md) | Create and manage Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse resources |
| [observability.md](observability.md) | Node-local metrics and log storage, federated queries, and the alert engine |
| [multi-node.md](multi-node.md) | Add and manage additional nodes, node health, and simple spread placement |
| [projects-and-organizations.md](projects-and-organizations.md) | The optional organization/project/environment grouping hierarchy for apps and databases |
| [identity-and-access.md](identity-and-access.md) | Users, roles, abilities, IAM policies, invites, tokens, 2FA, OAuth, and audit logging |
| [git-integrations.md](git-integrations.md) | Connect GitHub, GitLab, and Bitbucket, webhooks, and preview environments |
| [backups-and-storage.md](backups-and-storage.md) | Backup targets, registry credentials, and app volume backups |
| [templates-and-registry.md](templates-and-registry.md) | Deploy curated service templates from the catalog as Compose-backed apps |

### Reference

| Doc | Covers |
| --- | --- |
| [app-spec-reference.md](app-spec-reference.md) | Every `app.yaml` field, validated against `internal/spec`'s JSON Schema |
| [feature-catalog.md](feature-catalog.md) | Every dashboard page, API resource group, and CLI command group, plus known UI gaps |
| [cli-reference.md](cli-reference.md) | Every `levelrail` CLI command, organized by command group, extracted from source |
| [api-reference.md](api-reference.md) | Every REST route (272 total) grouped by resource, with ability and handler |

### Explanation

| Doc | Covers |
| --- | --- |
| [architecture.md](architecture.md) | How Levelrail is actually built today: reconciler, ingress, builds, storage |
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

## Support and contributing

- **Bug or question?** Open an issue on [GitHub](https://github.com/glincker/levelrail/issues).
- **Security vulnerability?** Don't open a public issue; see [Security overview](security.md#reporting-a-vulnerability) or the repository's [SECURITY.md](../SECURITY.md).
- **Want to contribute code?** See the repository's [CONTRIBUTING.md](../CONTRIBUTING.md) for branch naming, commit conventions, and how to run tests and the linter before opening a PR.

## Adding a new doc

1. Pick the Diátaxis type first (see the table above), not the package. Mixed reference and tutorial prose is the most common way docs rot, because neither reader gets what they need.

2. Add it to the matching heading in the Index section above.

3. Link it from the root README only if it is something a new user or contributor would hit early. Leave specialized how-tos and reference pages reachable only from here, so the root README stays focused.
