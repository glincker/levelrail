# ADR 031: How Levelrail integrates with outside systems

Status: Accepted. Refined by [ADR 034](034-plugin-contract-and-marketplace.md): "no plugin runtime" is replaced by a defined out-of-process boundary.

Date: 2026-10-11

## Context

Requests to "integrate with AWS", "list on a marketplace" or "support
plugins" keep arriving as separate asks. Each one is the same question:
where does an outside system touch the control plane, and with what
authority. The repo already answers that in practice, but the answer is
spread across docs and code and has never been written down as a rule.

What exists today:

- Sources: Git providers (GitHub, GitLab, Bitbucket, Gitea) in
  `internal/githubapp`, `internal/gitlabapp`, `internal/bitbucketapp`,
  `internal/giteaapp`, with inbound webhooks in `internal/webhook`.
  External image registries through stored registry credentials
  (`registryCredential` in `app.yaml`). `internal/registrycatalog` only
  browses Levelrail's own built-in registry, by design.
- Targets: node providers in `internal/provision` (Hetzner, DigitalOcean,
  AWS EC2, Azure, GCP), which create a VM and let the agent dial home.
  DNS at Cloudflare and Route53 in `internal/dnszones` and
  `internal/dnsrecords`. S3 compatible backup targets and storage
  destinations (AWS S3, R2, B2, Wasabi, MinIO, custom endpoint).
- Templates: the built-in catalog compiled into the binary, plus custom
  templates saved from a running app (ADR 015).
- Events: alert notification channels (`internal/alerting`), deploy
  notifications, log drains, and the audit stream exported through data
  export (`audit_webhook`, `audit_s3`, NDJSON or CSV download).
- Automation: scoped API tokens, `levelrail-cli`, and the MCP server
  (`internal/mcptools`, `cmd/levelrail-mcp`).

The control plane holds the master key, every stored credential, and the
reach to every node. That is the constraint every decision below follows
from.

## Decision

1. There are five integration points. A new integration must fit one, or
   it needs its own ADR.
   - Sources: where code and images come from (Git providers, registries).
   - Targets: where capacity and names live (node providers and clouds,
     DNS providers, S3 compatible storage).
   - Templates: what can be deployed in one click (built-in catalog,
     custom templates, and a reviewed community repository, see below).
   - Events: what the platform tells the outside world (notification
     channels, webhooks, log drains, the audit stream).
   - Automation: how outside programs and assistants drive the platform
     (API tokens, the CLI, MCP).

2. Integrations run out of process. They talk to Levelrail through API
   tokens (inbound) and events (outbound). There is no in-process plugin
   mechanism: no Go plugins, no embedded scripting runtime, no loading of
   third party code into the control plane. The control plane is
   privileged and holds secrets, so any code that runs inside it inherits
   the master key. Anything that acts on the platform authenticates as a
   token with the narrowest ability set that works and lands in the audit
   log like every other caller.

3. Provider support is added as a native client in `internal/provision`,
   `internal/dnszones` or the S3 layer, written and reviewed in this
   repo, not as a third party extension. A provider is listed as
   supported in the docs only once a real account has exercised it.
   Clients tested only against fakes are documented as such, as
   `docs/node-provisioning.md` already does for Azure and GCP.

4. Community templates, trust and review model. A template is a Compose
   file, so it is arbitrary code that runs on the operator's node.
   - A community template repository is a separate Git repository, not a
     runtime fetch from arbitrary URLs. Entries land by pull request.
   - Every entry is reviewed for: pinned image tags or digests, no
     privileged mode or host mounts without a stated reason, no
     hardcoded secrets (generated through the `$SERVICE_...` magic
     variables), a documented upgrade path, and a license the project
     can redistribute.
   - The built-in catalog stays compiled into the binary. Community
     entries are never executed or fetched by the control plane without
     an explicit operator action, and the full Compose body is shown
     before anything starts, as it is today.
   - A template deploys as an ordinary Compose app under the operator's
     own ability checks. It gets no additional authority.
   The community repository does not exist yet. This ADR fixes the model
   it must follow when it is built.

5. Explicitly not done:
   - No AWS Marketplace (or other cloud marketplace) listing until there
     is real demand from operators who would buy through one. A listing
     adds metering, billing and support obligations that a self-hosted
     Apache 2.0 project has no use for yet.
   - No plugin runtime, no extension API that executes third party code
     in the control plane or agent.
   - No Kubernetes CRDs, operators or Helm charts as a control surface.
     ADR 002 keeps the reconciler on the Docker Engine API, and a CRD
     layer would be a second source of desired state.

## Rejected alternatives

- In-process plugins (Go `plugin`, WASM, embedded Lua or JS). Rejected:
  the plugin would run with the control plane's master key and node
  reach. Sandboxing it well is its own project, and the single static
  binary (ADR 001) gets harder to build and cross compile. Tokens plus
  events cover the same ground with an existing audit trail.
- Terraform provider and Kubernetes operator as the primary interface.
  Rejected for now: both would sit on the same HTTP API, so the API is
  the product surface and wrappers can follow demand. Terraform snippets
  that call the existing API and CLI are on the planned list instead.
- Fetching templates live from any URL an operator pastes. Rejected: it
  turns a one-click install into remote code execution on a node with no
  review step. A reviewed repository keeps the trust boundary visible.
- A hosted template marketplace with ratings and publishers. Rejected:
  needs accounts, moderation and a hosted service, none of which fit a
  self-hosted tool at this stage.
- Cloud marketplace images (an AMI or GCE image with Levelrail
  preinstalled) now. Rejected until demand: it needs per-region image
  builds and an update policy. The install script on a fresh VM covers
  the same need.
- One generic "cloud" abstraction that hides provider differences.
  Rejected: credential shapes differ (AWS static key, role or instance
  profile; Azure service principal; GCP service account JSON), and the
  node provisioning docs are more useful stating each one plainly.

## Consequences

- New integration requests are triaged against the five points. Most
  resolve to "use a token and an event", which needs no control plane
  change.
- Planned cloud work stays inside existing seams: a Google Cloud DNS
  client next to Cloudflare and Route53, GCS through the S3
  interoperability API as a storage destination, an Artifact Registry
  preset and an ECR token refresh preset on top of registry credentials,
  and install script snippets for EC2 user-data and GCE startup scripts.
  `docs/cloud-integrations.md` tracks which of these exist.
- The community template repository is a prerequisite the project has
  not built. Until it exists, the only template extension path is a
  custom template saved from a running app.
- Operators who want a custom integration write a small service that
  holds a token and subscribes to webhooks or audit export. That service
  is theirs to run and secure.
