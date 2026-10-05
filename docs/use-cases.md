---
title: Who Levelrail is for
description: "Three real audiences for Levelrail (solo developers, small agencies, teams migrating off Coolify, Dokploy, Heroku, or Vercel), what each is likely trying to do, which shipped features actually fit, and where the project is not there yet."
---

# Who Levelrail is for

This page is not a marketing segmentation exercise. The audiences below come
straight out of the project's own stated scope: the
[README](https://github.com/glincker/levelrail#readme) describes Levelrail
as built for 3 to 50 services across 1 to 10 machines, explicitly not a
Kubernetes competitor, and [comparison.md](comparison.md#choosing) frames
the project the same way: an agent-based control plane with built-in
observability for someone who does not want to run Kubernetes to get there.
The three groups below are the ones that framing actually describes.

## How to read this page

- Every feature named below links to the doc page that covers it, and every
  maturity claim matches the label in [feature status](feature-status.md).
  Some areas are stable (backed by a live end-to-end test) and others are
  beta or behind a flag. Treat "it fits" as "the feature exists and is
  tested," not as "it has survived production load."
- Levelrail has no stable release yet. There is no stable image tag yet: `:latest`
  does not move, and prereleases publish as `:beta`. APIs, the app spec format, and the on-disk layout can still
  change without notice. None of the three audiences below should put a
  revenue-critical workload on it without a shadow run alongside whatever
  they use today, the same caution [migrating-from-vercel.md](migrating-from-vercel.md)
  gives explicitly.
- Where a claim is uncertain, this page says so instead of guessing, the same
  convention [comparison.md](comparison.md) uses.

## Solo developers and indie founders running their own stack

### What this group is likely trying to do

Run a handful of side projects or a small product on a VPS or two, without
paying per-service platform fees and without hand-rolling a deploy script
every time something new needs HTTPS and a database.

### Where Levelrail fits

- **One binary, single node.** The control plane and the agent can run in
  the same process over an in-memory transport
  ([architecture.md](architecture.md#node-agent)), so there is nothing extra
  to install for a one-server setup. This is also the best-tested path:
  per the [README status section](https://github.com/glincker/levelrail#status),
  "single node is the well-tested path."
- **Push to deploy, with a real health gate.** A new container only takes
  traffic after its readiness probe passes, not just after the process
  starts, and the previous N images stay pinned so a rollback target can
  never be garbage collected before you need it (see
  [comparison.md](comparison.md#where-levelrail-differs)).
- **TLS, metrics, and logs without installing anything else.** Caddy is
  embedded for automatic certificates, and every node keeps its own metrics
  (15 second resolution) and full-text-searchable logs out of the box
  ([architecture.md](architecture.md#observability)). A solo operator does
  not need to stand up a separate Grafana or log shipper to see why
  something was slow.
- **A template catalog.** Postgres, Redis, and about 300 other
  one-click services exist today for standing up the database or tool a
  side project needs ([templates-and-registry.md](templates-and-registry.md)).
- **Nothing paywalled.** OAuth sign-in, two-factor auth, audit logging, and
  scheduled backups with retention all ship in the same Apache 2.0 binary
  ([comparison.md](comparison.md#sso-audit-iam-and-backups-are-not-paywalled)),
  so a project that grows past "just me" does not hit a feature wall.

### What to know before you rely on it

- The measured idle footprint (58 to 96 MB RSS from 0 to 500 apps,
  [performance.md](performance.md)) is from a macOS dev build on one
  developer's machine, not a Linux release binary. Expect different numbers
  on your actual VPS, and no containers were running in that benchmark.
- TLS defaults to an internal, self-signed issuer. The public ACME issuer
  has one recorded live run against Let's Encrypt on a public VPS, but
  renewal and DNS-01 wildcards are not yet verified
  ([architecture.md](architecture.md#ingress), [acme-verification-runbook.md](acme-verification-runbook.md)).
  If your side project needs a browser-trusted cert today, test that path
  yourself before depending on it.
- Maturity varies by feature, per [feature-status.md](feature-status.md).
  Deploy approvals, IAM, PITR and database backups for Postgres, pipelines,
  deploy freeze and GitHub-based previews are labeled stable; load
  balancing, platform-as-code, the in-app AI chat, AI models and Cloudflare
  Tunnel are `hide-behind-flag` and stay off unless you enable them with
  `APP_EXPERIMENTAL`.

## Small agencies and freelancers running several clients' apps

### What this group is likely trying to do

Operate a handful of client projects on infrastructure the agency controls
(not a reseller platform with its own billing), give a client or contractor
scoped access to only their own app, and keep a record of who deployed what.

### Where Levelrail fits

- **Scoped access without a full RBAC build-out.** IAM policies are
  Allow/Deny statements scoped to `app:name`, `database:name`, or `*`, on
  top of three role presets (admin, operator, viewer), so a contractor's
  token can be restricted to exactly one client's app
  ([identity-and-access.md](identity-and-access.md)). An explicit Deny
  always wins, even over `root`.
- **A queryable audit log.** Every mutating and sensitive request is
  recorded with actor, ability, method, path, and caller surface (CLI,
  dashboard, MCP, or API), exportable as CSV
  ([comparison.md](comparison.md#sso-audit-iam-and-backups-are-not-paywalled)).
  Useful for an agency that needs to show a client what changed and who did
  it.
- **Organizational labels for grouping client work.** Projects,
  organizations, and environments group apps and databases and layer shared
  env vars, though per
  [projects-and-organizations.md](projects-and-organizations.md) these are
  plain organizational labels today: no per-project membership or
  permissions yet, since that layer is carried by IAM policies instead.
- **A read-only importer for onboarding an existing client.** Moving a
  client off Coolify, Dokploy, or CapRover runs a dry run first and only
  issues GET requests against the source
  ([migrating-from-coolify-dokploy-and-caprover.md](migrating-from-coolify-dokploy-and-caprover.md)).
- **Dedicated per-client nodes if you outgrow one box.** Cloud node
  provisioning covers five providers (Hetzner, DigitalOcean, AWS, Azure,
  GCP), so a client that needs isolation can get its own node
  ([node-provisioning.md](node-provisioning.md), [multi-cloud-provisioning.md](multi-cloud-provisioning.md)).

### What to know before you rely on it

- IAM is labeled stable: a live end-to-end test shows a Deny policy
  blocking a real action and a token allow policy scoping one
  ([feature-status.md](feature-status.md#iam)). Policy enforcement is wired
  into specific app and database routes, so still verify a client's scoped
  token cannot touch another client's app before trusting that boundary.
- Multi-node enrollment (cordon, drain, container relocation) has been
  verified locally across two real Docker daemons, but the WireGuard mesh
  itself and the cross-host remote transport remain unverified, and this
  has not been run across a real WAN or a second physical host
  ([feature-status.md](feature-status.md#multi-node-and-wireguard-mesh)).
  If "a client's node is in a different datacenter" is part of the plan,
  treat that as the least-proven part of the setup.
- There is no billing, metering, or customer-facing subscription layer.
  This is infrastructure the agency operates for its own clients, not a
  platform for reselling hosting with its own payment flow.
- There are no teams or org-level membership rules yet; access control runs
  entirely through IAM policies and roles on individual users and tokens,
  not through a team hierarchy.

## Teams moving off Coolify, Dokploy, Heroku, or Vercel

### What this group is likely trying to do

Already run on one of those platforms, and want more control over how
servers are driven, or hit a limitation (SSH-based management, a paywalled
feature, a managed-cloud pricing model) that Levelrail's architecture
avoids.

### Where Levelrail fits

- **A documented importer for Coolify, Dokploy, and CapRover.** Reads the
  source platform's own API, supports a dry run that reports what would be
  created without creating anything, and is read-only against the source
  ([migrating-from-coolify-dokploy-and-caprover.md](migrating-from-coolify-dokploy-and-caprover.md)).
- **A concept-mapping runbook for Vercel.** There is no importer for
  Vercel; [migrating-from-vercel.md](migrating-from-vercel.md) is an
  explicit manual runbook with an inventory step, a Vercel-to-Levelrail
  concept table, and named gaps (Edge runtime has no equivalent, for
  example) rather than a pretense that everything maps cleanly.
- **The architectural reasons to move**, laid out without ranking the
  competition: an agent that dials out over mTLS instead of SSH or Docker
  Swarm, a level-triggered reconciler instead of an imperative script,
  metrics and logs built into the core instead of a bolt-on, and rollback
  images pinned against garbage collection
  ([comparison.md](comparison.md#where-levelrail-differs)).
- **An MCP server for AI tooling**, if that is part of why Heroku or
  Vercel's own AI integrations feel limited: the same HTTP API surface is
  exposed to AI agents, with AI explicitly never in the reconcile path
  ([comparison.md](comparison.md#where-levelrail-differs), labeled beta,
  118 to 156 tools depending on mode, in
  [feature-status.md](feature-status.md#ai-assistant-mcp-server-and-in-app-chat)).

### What to know before you rely on it

- Heroku has no dedicated migration runbook. Only
  Coolify/Dokploy/CapRover (importer-backed) and Vercel (manual runbook)
  are covered. [Heroku alternative](heroku-alternative.md) gives the
  overview, and a move means working from the general
  [architecture](architecture.md) and [app spec reference](app-spec-reference.md)
  docs.
- Template catalogs differ between platforms. Levelrail ships 311, and
  only a sample is deployed by its end-to-end tests, so check that the
  services you need exist and boot (see [templates-and-registry.md](templates-and-registry.md)
  and [feature-status.md](feature-status.md#templates)).
- `docs/migrating-from-vercel.md` itself warns to run the new deployment in
  parallel and keep the old platform as the rollback path until a shadow
  run is clean, specifically because there is no stable release yet and
  public ACME renewal is unverified. That caution applies to
  anyone moving off any existing platform, not just Vercel.
- Preview environments are stable for GitHub but beta for GitLab and
  Bitbucket ([feature-status.md](feature-status.md#previews)), so a team
  that lives on GitLab should expect rougher edges there than a GitHub-based
  team would.

## Questions that come up across all three groups

**Can I use this to resell hosting to my own customers, with billing?**
No. There is no billing, metering, or subscription layer anywhere in this
codebase. IAM policies can scope a customer's access to their own app, but
charging them is outside this project's scope.

**Is this ready to replace my production Heroku/Vercel/Coolify setup today?**
Not without caution. There is no stable release yet (README status
section), and the recommended path, explicit in
[migrating-from-vercel.md](migrating-from-vercel.md), is to run Levelrail
alongside your existing platform until a shadow run is clean.

**Do I need to run more than one server?**
No. Single node is the default and the best-tested path. A second node is
something you add when one box runs out of room, not something the
platform assumes from day one ([multi-node.md](multi-node.md)).

**What if I only need one of these three use cases halfway?**
That is fine. These are the audiences the project's own non-goals point at,
not a rigid segmentation. Read [architecture.md](architecture.md) and
[feature-status.md](feature-status.md) directly and decide which parts
apply to your actual setup.

## See also

- [Comparison](comparison.md) - how Levelrail's architecture differs from
  Coolify, Dokploy, CapRover, Dokku, and Kamal
- [Feature status](feature-status.md) - the maturity label behind every
  claim on this page
- [Migrating from Coolify, Dokploy, or CapRover](migrating-from-coolify-dokploy-and-caprover.md)
- [Migrating from Vercel](migrating-from-vercel.md)
- [Roadmap](roadmap.md) - what has shipped and what is open
