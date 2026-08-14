# Creation wizard + sidebar IA

## Problem

Sidebar primary nav only shows Apps and Databases (Nodes exists but is
buried under Settings). Creating an app or database is a single-step
modal (name/image/port, or name/engine). Spinning up a real container on
someone's own VPS deserves more than that, and Coolify's own "New
Resource" flow (studied via the local read-only clone in
`docs-local/competitor-clones/coolify`) is a good reference: a small,
fixed picker of starting points, not a big catalog.

Confirmed from that clone: `app/Enums/NewResourceTypes.php` is a 15-entry
fixed enum (git sources, Dockerfile, Compose, Docker image, a service
catalog, 8 database engines), and `public/svgs/resources/` holds exactly
18 curated brand-logo SVGs for those types, not hundreds. This matches
CLAUDE.md section 2's own non-goal: "Not chasing Coolify's 280 one-click
templates. Ten good ones beat 280 stale ones," and the full curated
template catalog is explicitly Phase 4 work (section 6), not now.

## Design

### 1. Sidebar

Promote the existing `/nodes` route from the Settings group to primary
nav, alongside Apps and Databases. It's a real resource kind (multi-node
placement), not admin config, the same category as the other two. No new
backend resource kind is being invented to satisfy "more sections":
Nodes already exists and was just under-promoted.

### 2. Creation flow: a 2-step wizard, still a Dialog

Not a dedicated route: Coolify's own new-resource flow is a panel, not a
full page either, and a dialog keeps the "create without losing your
place" property the current modals already have.

**Step 1 — pick a starting point.** A small grid, four options, all
backed by code that already works:

- Docker image (today's existing `CreateAppDialog` path, kept as-is)
- Dockerfile from git (routes to the manual build-trigger path just
  merged, `POST /apps/{name}/builds`)
- Postgres (today's existing `CreateDatabaseDialog` path)
- Redis (same)

MySQL was the original fifth pick but `internal/reconcile/database/controller.go`
only handles `postgres`/`redis` today (verified by grep, no MySQL case
anywhere in that controller). Adding a third engine is reconciler-core
per CLAUDE.md section 8 (single-session, not parallelized), so it's a
fast-follow, not part of this wizard's first cut: shipping a picker
option backed by nothing would be worse than one fewer option.

**Step 2 — minimal config for whichever was picked.** Reuses each path's
existing field set (name/image/port; name/repo/ref; name/engine version).
No new backend work for the image or database paths; the Dockerfile+git
option is new frontend wiring onto the endpoint that already exists.

**Step 3** is unchanged: Create/Deploy lands on the resource's detail
page, same as today.

This is "3-click start" in Coolify's own sense: pick a type (click),
fill 1-2 required fields, click Deploy/Create.

### 3. Icons

Two different concerns, resolved differently:

- **UI chrome** (nav, buttons, dialog controls): stays 100% Phosphor
  (`@phosphor-icons/react/dist/ssr`). Locked by CLAUDE.md and ADR 014.
  Not in scope for change here.
- **Per-service brand logos** in the step-1 picker (Docker whale,
  a generic git/Dockerfile mark, Postgres elephant, Redis cube): sourced
  from thesvg.org, confirmed to carry real brand/tech logos, not just
  general UI iconography. These are vendored as a handful of static SVG
  files (4, matching the 4 picker options), not an npm package
  dependency and not a new React icon component library — the same
  shape Coolify's own `public/svgs/resources/` folder already uses.
  This keeps the "don't stuff heavy assets" concern satisfied (a
  handful of small files, not a whole library pulled in) without
  reopening the Phosphor-only rule, which is specifically about the
  app's own UI iconography system.

## Out of scope

- A full template catalog (Phase 4, non-goal for now).
- Any new backend resource kind beyond what already exists (apps,
  databases, nodes).
- Changing how Docker image or database creation actually works;
  this only adds a step-1 picker in front of existing paths plus one
  new frontend wiring for the git-build path.
