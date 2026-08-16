# ADR 015: Reverse the "no one-click template catalog" non-goal

Status: Accepted

Date: 2026-08-16

## Context

Section 2's non-goals list has said, since Phase 0, that a large
one-click service-template catalog was out of scope: a small, curated
set of resource types beats a large, stale one. The founder shared a
screenshot of Levelrail's own "New resource" modal (five plain cards:
Docker image, Dockerfile from git, Postgres, Redis, MySQL) next to a
competing self-hosting platform's own modal (categorized Applications/
Databases/Services, real icons, search, a live services catalog),
calling the current one "very limited," and asked for a real,
categorized template catalog.

A real, publicly available service-template dataset (hundreds of
entries, each a Docker Compose file) was reviewed locally to understand
what a catalog like this actually needs. Two materially different asks
could satisfy the founder's complaint: (a) a UX/IA overhaul of the
existing picker for Levelrail's own ~9 resource types (no non-goal
conflict), or (b) actually importing a real, large template catalog (a
direct reversal of the non-goal above). Asked directly; the founder
chose (b) explicitly.

A second, load-bearing fact surfaced while scoping the work: every one
of those templates is a Docker Compose file, and Levelrail's reconciler
(`internal/reconcile/application`) manages exactly one container per
service today, with no Compose-as-a-deploy-target support at all. Asked
about sequencing; the founder chose to have Compose support built
first, as dedicated reconciler-core work, before templates or picker
UI.

## Decision

Reverse the non-goal. Importing a real service-template catalog is in
scope, but gated on Docker Compose support landing first:

1. Docker Compose as a deploy target: a compose file's `services:` fan
   out into multiple `store.DesiredService` rows sharing one
   `store.App` (`migrations/0039_apps.sql`'s stage-1 grouping), each
   still reconciled independently by the existing single-container
   `Controller`, no new reconciler paradigm needed.
2. Named-volume support for ordinary application services (previously
   database-controller-only), since most real templates need
   persistent data to be useful at all. Landed independently as its
   own PR (`store.DesiredService.Volumes`, `spec.Service.Volumes`).
3. Cross-service name resolution within a compose stack: solved by
   either mesh DNS or per-app Docker networking (whichever a service's
   deployment has enabled), not by anything specific to Compose
   ingestion itself.
4. Once Compose deploys work end to end, translate a curated set of
   real templates into Levelrail's own deploy flow, and build the
   categorized, richer "New resource" picker UI on top.

## Rejected alternatives

- **UX-only overhaul, no template import.** Rejected: the founder was
  offered this as the lower-cost option and explicitly chose the full
  catalog import instead.
- **Import template metadata now, stub the Deploy button until Compose
  support lands.** Considered as a middle path (offered directly); not
  chosen. The founder chose to build Compose support first and have
  templates/UI depend on real, working deploys rather than ship a
  picker that looks complete but doesn't actually work yet.
- **Build Compose support without volumes, ship a smaller MVP faster.**
  Rejected once flagged: a large share of real templates (anything
  with its own data directory, which is most of them) would silently
  lose data on every restart without volume support, an unacceptable
  default for a deploy platform. The founder chose to build volume
  support as part of this work rather than defer it.

## Consequences

- Section 2 of `CLAUDE.md` now marks the "no large template catalog"
  bullet as reversed, pointing here, rather than deleting it outright:
  the original reasoning (avoid stale, untested templates) still
  matters and should inform how the real catalog gets imported
  (curated, tested, not a blind bulk copy), even though the "don't do
  it at all" conclusion no longer holds.
- Compose support is real, non-trivial reconciler-adjacent work
  (`internal/compose` parser, fan-out into `store.App` + member
  services, volume/network wiring).
- A real survey of a public template dataset found the large majority
  of entries rely on a competing platform's own auto-generated
  password/domain env-var convention, not plain literal values; making
  those genuinely one-click needs a resolver for that convention, not
  just a compose parser. That resolver is separate, follow-up work,
  not yet built.
- No commitment yet to importing an entire third-party catalog
  verbatim; the actual import strategy (full catalog vs. curated
  subset, how compose-file quirks outside this platform's supported
  subset get handled) is a separate decision once Compose support
  itself is real and tested.
