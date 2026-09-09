# Levelrail docs

This directory is the source of truth for Levelrail's user-facing and
contributor-facing documentation. Two things are true about it on purpose:

- **It ships with the repo, not the binary.** Nothing under `/docs` is
  `embed.FS`'d into the control plane binary or the Docker image (unlike
  `web/`'s built frontend assets, see `CLAUDE.md` section 4.1). It's
  read on GitHub today; if it ever moves to a hosted docs site
  (`glinr.com` or elsewhere), that's a publishing step on top of these
  files, not a rewrite of them.
- **It's plain Markdown, deliberately.** No MDX, no build-tool-specific
  syntax, no frontmatter tied to one platform's schema. Markdown "renders
  anywhere" (GitHub, a future static site generator, a README preview, an
  AI agent reading the repo raw) without conversion. When a specific
  target platform is chosen, that platform's own frontmatter fields
  (`sidebar_position`, `layout`, whatever it needs) get added on top of
  this content then, not guessed at now.

## How this index is organized

Docs here follow the [Diátaxis](https://diataxis.fr) framework: organize
by what the reader is trying to do, not by which package the content
happens to describe. Four types, plus two Levelrail-specific categories
that don't fit Diátaxis's four (a status page and pre-ADR design
proposals):

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
| [getting-started.md](getting-started.md) | Build and run the control plane and agent locally, deploy a first app |

### How-to guides

| Doc | Covers |
| --- | --- |
| [installing.md](installing.md) | Every install path (`install.sh`, Docker, source), verifying, upgrading, and uninstalling |
| [docker.md](docker.md) | Run the control plane and node agent as containers instead of `install.sh` |
| [feature-flags.md](feature-flags.md) | Toggle app behavior at runtime without a redeploy |
| [screenshots.md](screenshots.md) | Regenerate the dashboard screenshots used in the README |
| [master-key-rotation.md](master-key-rotation.md) | Rotate the envelope-encryption master key without losing access to stored secrets |
| [migrating-from-coolify-and-dokploy.md](migrating-from-coolify-and-dokploy.md) | Move apps off a live Coolify or Dokploy instance with `levelrail-cli migrate` |
| [domains-and-ingress.md](domains-and-ingress.md) | Why there's no reverse proxy to install, how `app.yaml` domains route to containers, and TLS's current honest status |
| [acme-verification-runbook.md](acme-verification-runbook.md) | Verify real ACME certificate issuance against a live domain, step by step |

### Reference

| Doc | Covers |
| --- | --- |
| [app-spec-reference.md](app-spec-reference.md) | Every `app.yaml` field, validated against `internal/spec`'s JSON Schema |
| [feature-catalog.md](feature-catalog.md) | Every dashboard page, API resource group, and CLI command group, plus known UI gaps |

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

## Adding a new doc

1. Pick the Diátaxis type first (see the table above), not the package
   it happens to describe: a reference page mixed with tutorial prose
   is the most common way docs rot, because neither reader gets what
   they came for.
2. Add it to the Index section above, under the matching heading.
3. Link it from `README.md`'s own docs section if it's something a new
   user or contributor would hit early; leave more specialized how-tos
   and reference pages reachable only from here, so the root README
   doesn't grow into a second index.
