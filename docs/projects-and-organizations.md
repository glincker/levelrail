# Projects, organizations, and environments

The grouping hierarchy for apps and databases. Package:
`internal/api/projects.go`, `organizations.go`, `environments.go`,
`project_env.go`, `organization_env.go`, `environment_env.go`,
`project_stop_start.go`, `project_restart.go`.

## Why this exists

An app or a database is a real, running thing with its own lifecycle.
A project is not: it is purely an optional label an app or database can
be filed under (`internal/api/projects.go`'s own package comment).
There is no owner, no member list, no per-project permission of any
kind. The single admin user sees every project and every app/database
regardless of membership, exactly like it already sees everything else.
This is deliberate: the repo plan's Phase 4 note about "teams, projects,
environments, RBAC" is explicitly *not* what this is. Projects, and the
organizations that group them, arrived early as organizational labels;
RBAC is a separate, later concern.

The hierarchy is:

```
organization (optional)
  └── project (optional)
        ├── environment (optional, e.g. staging / production)
        └── app / database (filed under the project directly)
```

An app or database can skip every level and belong to nothing. Nothing
about how it runs changes based on where it's filed. Deleting a
project, an organization, or an environment never deletes or disrupts
what was filed under it: every foreign key involved
(`desired_services.project_id`, `desired_databases.project_id`,
`projects.org_id`, `desired_services.environment_id`) is `ON DELETE SET
NULL`, so the member just becomes unlabeled again.

Environments sit one level below a project, not below an organization:
an environment is scoped to exactly one project
(`GET/POST /api/v1/projects/{id}/environments`), and only an app can be
tagged with one, not a database. That last asymmetry isn't an oversight,
it falls out of what environments are actually for: the protected-
environment confirmation gate (see below) exists to slow down a
deploy/rollback/promote, and none of those three actions apply to a
database.

## How the grouping actually works

Both a project and an organization are addressed by ID, never by name:
`internal/api/projects.go`'s own comment explains that project names
are deliberately non-unique for this reason, so there is no
duplicate-name rejection the way `POST /apps` rejects a duplicate app
name. Moving an app or database into a project, or an app into an
environment, is a separate PUT call against the app/database itself,
not a field on its create/update body:

- `PUT /api/v1/apps/{name}/project` and
  `PUT /api/v1/databases/{name}/project` move a resource into (or with
  `project_id: ""`, out of) a project.
- `PUT /api/v1/apps/{name}/environment` tags (or, with
  `environment_id: ""`, untags) an app with an environment.
- `PUT /api/v1/projects/{id}/organization` files (or, with `org_id:
  ""`, unfiles) a project under an organization.

Every one of these validates the target ID against the real registry
first: a typo'd or already-deleted project/environment/org ID gets a
400, not a silent write. This mirrors the exact shape
`PUT /apps/{name}/node` already established for node placement,
just one level up in the resource hierarchy, and it's gated by
ordinary `AbilityWrite`, not `AbilityRoot`: filing something under a
project is an organizational edit, not an infrastructure change.

On the dashboard, this move happens from the resource's own Overview
page, not from the project/environment page: `AppOverview.tsx` renders
`MoveToProjectDialog` and `MoveToEnvironmentDialog`, and
`routes/databases/$name/overview.tsx` renders `MoveToProjectDialog` for
a database. The project and environment detail pages
(`routes/projects/$id/index.tsx`, `routes/projects/$id/environments/$envId.tsx`)
are read-only with respect to membership: they list what's already
filed there and link back to each member's own Overview page to move
it. An organization is filed onto a project the same way, in reverse:
`MoveToOrganizationDialog` lives on the project detail page, not the
organization's.

There is no server-side filtered listing endpoint like
`GET /api/v1/projects/{id}/apps`. The project detail page filters the
same full `GET /api/v1/apps` and `GET /api/v1/databases` responses the
unfiltered list pages already fetch, by each row's own `project_id`
field. The organization detail page does the identical thing to
`GET /api/v1/projects` by `org_id`. This keeps the "additive
organization, not a forced migration" principle honest: neither `/apps`
nor `/databases` nor `/projects` changes shape because this grouping
exists, and a page that already warmed the unfiltered list pays no
second fetch.

## Shared env var layering

A project, an organization, and an environment can each hold their own
set of shared env vars (`GET`/`PUT .../env`, a plain
`map[string]string`, full-replace on write, the same semantics
`PUT /apps/{name}`'s own `env` field has). These sit *beneath* an app's
own `env`/`secretEnv` in `resolveEnv`
(`internal/reconcile/application/controller.go`), applied lowest tier
first so anything above it can freely override a same-named key:

```
organization env   (lowest, applied first)
  → project env
    → environment env
      → this app's own literal env
        → this app's own secret-backed env      (overrides a literal)
          → attached storage target's S3_* env  (overrides all of the above)
            → attached database's connection env (highest, applied last)
```

In practice, an app only ever sees the organization/project/environment
tiers that actually apply to it: the organization tier only resolves
when the app has a `project_id` *and* that project is filed under an
organization; the environment tier only resolves when the app has an
`environment_id`. An app with no project and no environment just gets
its own `env`/`secretEnv` (and any storage/database env), unchanged
from before this layering existed.

This is why environments hang off a project rather than an
organization directly: an environment's shared vars need a project's
own vars beneath them to override, and a project's vars need an
organization's beneath those, so the layering only makes sense in that
fixed order.

## Moving a resource between projects

There is no dedicated "move" endpoint distinct from the assignment
endpoint: moving is the same `PUT .../project` call as assigning, just
against a resource that already has one. The store write happens
immediately; nothing about the resource's running container, image, or
config changes. On the CLI:

```bash
levelrail-cli apps set-project my-app proj_abc123
levelrail-cli databases set-project my-db proj_abc123
levelrail-cli apps clear-project my-app
```

## Project-wide pause, resume, and restart

Stopping or starting a project acts on every app and every database
filed under it in one call: `POST /api/v1/projects/{id}/stop` sets
each member's `suspended` flag to `true`, `.../start` clears it, the
identical effect `POST /apps/{name}/stop` (or the database equivalent)
has on one resource, applied project-wide. Like a single stop/start,
this only flips the desired-state flag; the application and database
reconcilers are what actually stop or start containers on their next
pass.

Restart is app-only: `POST /api/v1/projects/{id}/restart` forces every
app filed under the project to have its running container recreated
with no image change (a fresh `restart_nonce`, the same mechanism
`POST /apps/{name}/restart` uses on one app). Databases have no bulk
restart route at this scope.

All three endpoints attempt every member independently and never abort
on one failure. The response reports which resources succeeded and
which failed, so an operator restarting or stopping a whole project's
worth of apps sees a partial result instead of one bad app blocking
every other one:

```json
{
  "succeeded_apps": ["api", "worker"],
  "succeeded_databases": ["main-postgres"],
  "failed_apps": ["flaky-app"]
}
```

On the dashboard, the project detail page shows a "Stop project" /
"Start project" pair (`PauseResumeProjectButton`, always both actions
rather than one toggle, since a database's `suspended` state isn't in
`GET /api/v1/databases`'s response yet, so there's no reliable
"is everything already stopped" signal to key a single toggle off) and
a "Restart all" button (`RestartProjectButton`, disabled when the
project has no apps). Neither is behind a confirm dialog: stopping is
not destructive, every desired-state row survives untouched, and a
restart is the same non-destructive action `RestartAppButton` already
performs without one.

## Protected environments

An environment created with `protected: true` requires an explicit
`confirm: true` on any of the three actions that change what a tagged
app runs: `POST /apps/{name}/deploys`, the promote endpoint
(`POST /apps/{name}/promote`), and rollback (which is the identical
deploy endpoint run with an older image tag, per
`handleTriggerDeploy`'s own doc comment: there is no separate rollback
mechanism). Without `confirm: true`, the server rejects the request
with `409 Conflict` and a message naming the environment:

```json
{
  "error": "environment \"production\" is protected; set confirm: true to proceed"
}
```

The check (`requireEnvironmentConfirmation`,
`environmentNeedsConfirmation` in `environments.go`) degrades safely on
either side: an app with no environment, or one tagged with an
unprotected environment, always passes with no friction at all; a
stale or already-deleted `environment_id` reference is treated as "not
protected" rather than blocking the caller with an error this endpoint
isn't responsible for validating.

On the CLI, `apps deploy`, `apps rollback`, and `apps promote` all take
a `--confirm` flag. Omitting it doesn't fail outright: the client
catches the 409, prints the server's own message, and prompts
interactively on stdin (`Type "yes" to proceed:`), the same
fail-closed-on-EOF behavior a scripted destructive restore already
uses. Typing anything other than `yes` (or hitting EOF, for a
non-interactive script) leaves the original 409 as the final result.

```bash
levelrail-cli apps deploy my-app --image registry.example.com/org/app:v2 --confirm
levelrail-cli apps rollback my-app --image registry.example.com/org/app:v1 --confirm
levelrail-cli apps promote my-app --to env_prod123 --confirm
```

On the dashboard, the deploy/rollback/promote flow surfaces
`ProtectedEnvironmentNotice`, an amber warning box with a checkbox
("I understand and want to proceed.") that must be checked before the
button enables, the same acknowledge-then-enable pattern used for
Redis's no-auth public-access warning. Toggling `protected` itself is a
`Switch` on the environment detail page (`ProtectedEnvironmentToggle`),
backed by `PATCH /api/v1/environments/{id}`, the only field that route
ever changes.

## Dashboard pages

- `/projects`: virtualized list of every project, with a "New project"
  dialog.
- `/projects/$id`: one project's name, its organization (with a "Move"
  action), its environments (`ProjectEnvironmentsPanel`, create/delete
  inline), stop/start/restart/delete actions, and its member apps and
  databases (client-filtered, as above; move or remove a member from
  its own Overview page).
- `/projects/$id/environments/$envId`: one environment's protected
  toggle, its own shared env var editor, links to sibling environments
  in the same project, and every app currently tagged with it.
- `/settings/organizations`: list of every organization, with a "New
  organization" dialog. File a project into one from the project's own
  detail page, not from here.
- `/organizations/$id`: one organization's name, its shared env var
  editor, and every project filed under it (client-filtered from
  `GET /api/v1/projects`).
- An app's or database's own Overview page: the "Move" (project) and,
  for apps, "Change" (environment) actions that actually reassign
  membership.

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/projects` | `read` |
| `POST` | `/api/v1/projects` | `write` |
| `GET` | `/api/v1/projects/{id}` | `read` |
| `DELETE` | `/api/v1/projects/{id}` | `write` |
| `POST` | `/api/v1/projects/{id}/stop` | `deploy` |
| `POST` | `/api/v1/projects/{id}/start` | `deploy` |
| `POST` | `/api/v1/projects/{id}/restart` | `deploy` |
| `GET` | `/api/v1/projects/{id}/env` | `read` |
| `PUT` | `/api/v1/projects/{id}/env` | `write` |
| `PUT` | `/api/v1/apps/{name}/project` | `write` |
| `PUT` | `/api/v1/databases/{name}/project` | `write` |
| `GET` | `/api/v1/organizations` | `read` |
| `POST` | `/api/v1/organizations` | `write` |
| `GET` | `/api/v1/organizations/{id}` | `read` |
| `DELETE` | `/api/v1/organizations/{id}` | `write` |
| `PUT` | `/api/v1/projects/{id}/organization` | `write` |
| `GET` | `/api/v1/organizations/{id}/env` | `read` |
| `PUT` | `/api/v1/organizations/{id}/env` | `write` |
| `GET` | `/api/v1/projects/{id}/environments` | `read` |
| `POST` | `/api/v1/projects/{id}/environments` | `write` |
| `PATCH` | `/api/v1/environments/{id}` | `write` |
| `DELETE` | `/api/v1/environments/{id}` | `write` |
| `PUT` | `/api/v1/apps/{name}/environment` | `write` |
| `GET` | `/api/v1/environments/{id}/env` | `read` |
| `PUT` | `/api/v1/environments/{id}/env` | `write` |
| `POST` | `/api/v1/apps/{name}/deploys` (needs `confirm: true` if protected) | `deploy` |
| `POST` | `/api/v1/apps/{name}/promote` (needs `confirm: true` if protected) | `deploy` |
| `GET` | `/api/v1/apps/{name}/promote/preview` | `read` |

## CLI

```bash
# Projects
levelrail-cli apps projects create --name NAME [flags]
levelrail-cli apps projects list [flags]
levelrail-cli apps projects get <id> [flags]
levelrail-cli apps projects delete <id> [flags]
levelrail-cli apps projects stop <id> [flags]
levelrail-cli apps projects start <id> [flags]
levelrail-cli apps projects restart <id> [flags]
levelrail-cli apps projects env-get <id> [flags]
levelrail-cli apps projects env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]

# Organizations
levelrail-cli apps organizations create --name NAME [flags]
levelrail-cli apps organizations list [flags]
levelrail-cli apps organizations get <id> [flags]
levelrail-cli apps organizations delete <id> [flags]
levelrail-cli apps organizations set-project <project-id> <org-id> [flags]
levelrail-cli apps organizations clear-project <project-id> [flags]
levelrail-cli apps organizations env-get <id> [flags]
levelrail-cli apps organizations env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]

# Environments
levelrail-cli apps environments create <project-id> --name NAME [--protected] [flags]
levelrail-cli apps environments list <project-id> [flags]
levelrail-cli apps environments update <id> --protected=true|false [flags]
levelrail-cli apps environments delete <id> [flags]
levelrail-cli apps environments env-get <id> [flags]
levelrail-cli apps environments env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]

# Moving apps/databases
levelrail-cli apps set-project <name> <project-id> [flags]
levelrail-cli apps clear-project <name> [flags]
levelrail-cli databases set-project <name> <project-id> [flags]
levelrail-cli databases clear-project <name> [flags]
levelrail-cli apps set-environment <name> <environment-id> [flags]
levelrail-cli apps clear-environment <name> [flags]

# Deploy/rollback/promote against a protected environment
levelrail-cli apps deploy <name> --image IMAGE [--confirm] [flags]
levelrail-cli apps rollback <name> --image IMAGE [--confirm] [flags]
levelrail-cli apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--confirm] [--preview] [flags]
```

## Not built yet (deliberate follow-ups)

- **No project-scoped or organization-scoped auth.** Filing something
  under a project or organization is a label, not a permission
  boundary; the single admin user sees and can act on everything
  regardless of what it's filed under. Real membership/RBAC is a
  separate, later piece of work (see the repo plan's Phase 4).
- **No bulk move.** Moving apps/databases between projects, or between
  environments, is one resource at a time; there is no "move every app
  in project A to project B" endpoint.
- **No database environment tagging.** Only an app can be tagged with
  an environment (`PUT /api/v1/apps/{name}/environment`); there is no
  equivalent route for a database, since the protected-environment gate
  only guards deploy/rollback/promote, none of which apply to a
  database.
- **No project-scoped or organization-scoped deploy history, audit
  view, or dashboard beyond the plain member list.** A create/update/
  delete against any of these three resources lands in the platform's
  existing generic audit log (`GET /api/v1/audit-log`) the same as any
  other authenticated write, with no dedicated per-project or
  per-organization view on top of it.
- **No server-side filtered listing.** `GET /api/v1/projects/{id}/apps`
  (or the organization/environment equivalents) doesn't exist; the
  dashboard filters the full unfiltered list client-side, as described
  above. Revisit only if that becomes a real scale problem for the
  3-50-service target audience this platform is built for.
