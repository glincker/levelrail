---
title: "Team and project quotas"
description: Cap how many apps, databases, previews and how much memory and CPU a team or project may hold, from the dashboard, the CLI or the API.
---

A quota is a ceiling on what one team or one project may hold. By default nothing is limited. Set a ceiling and any create or change that would go past it is refused with the limit and the current usage, so one team cannot starve the rest of the instance.

## What can be limited

| Limit | Counts | Field |
| --- | --- | --- |
| Apps | apps, not counting preview apps | `max_apps` |
| Databases | managed databases | `max_databases` |
| Memory | memory reservations of apps (times replicas) and databases, in MB | `max_memory_mb` |
| CPU | CPU reservations in cores, same rules as memory | `max_cpu` |
| Previews | live preview environments (deploying or active) | `max_previews` |

`0` or an unset field means unlimited. Memory and CPU count the limits you set on a resource, so an app with no memory limit reserves nothing against the quota.

A team quota covers every project filed under the team, plus anything with no project (the default team). A project quota covers only that project. A change must fit under both.

## Where quotas are enforced

| Action | Checked |
| --- | --- |
| Create an app or database | one more app or database, plus its memory and CPU |
| Raise an app's memory, CPU or replicas | the increase only |
| Raise a database's resources | the increase only |
| Move an app or database into another project | the destination project and, if the team changes, the destination team |
| Open a preview environment | one more preview |

A refused change answers `409` with code `quota_exceeded`:

```json
{
  "error": "project \"shop\" has reached its apps quota: 5 of 5 used, 1 more requested",
  "code": "quota_exceeded"
}
```

Landing exactly on a limit is allowed. Lowering a resource is always allowed, even while a scope is over its limit. Lowering a limit below current usage never stops anything that is running, it only blocks further growth.

Checks are serialized, so two concurrent creates cannot both take the last free slot.

## Set quotas

Open **Settings, Teams** for a team, or a project's page, and use the **Quotas** card. It shows a usage bar for every dimension and, for team admins and owners, a form to change the limits. Members see usage against the limit but cannot change it.

From the CLI:

```bash
levelrail quota show team org_acme
levelrail quota set project proj_shop --max-apps 5 --max-memory-mb 4096 --max-cpu 2
levelrail quota clear project proj_shop
levelrail quota show project proj_shop --json
```

`quota set` replaces all limits, so pass every limit you want to keep.

From the API:

| Endpoint | Action |
| --- | --- |
| `GET /api/v1/teams/{id}/quota` | limits, effective limits and usage |
| `PUT /api/v1/teams/{id}/quota` | replace the limits (team admin or owner) |
| `DELETE /api/v1/teams/{id}/quota` | remove the limits |
| `GET /api/v1/projects/{id}/quota` | same, for a project |
| `PUT /api/v1/projects/{id}/quota` | same, for a project |
| `DELETE /api/v1/projects/{id}/quota` | same, for a project |

Every change is written to the audit log and to the team's event trail as `quota.updated` or `quota.cleared`.

## Instance defaults

These environment variables set limits for every team and project that has no value of its own. All default to unlimited.

| Variable | Meaning |
| --- | --- |
| `APP_QUOTA_DEFAULT_MAX_APPS` | apps |
| `APP_QUOTA_DEFAULT_MAX_DATABASES` | databases |
| `APP_QUOTA_DEFAULT_MAX_MEMORY_MB` | memory reservation in MB |
| `APP_QUOTA_DEFAULT_MAX_CPU` | CPU reservation in cores |
| `APP_QUOTA_DEFAULT_MAX_PREVIEWS` | live previews |
| `APP_QUOTA_WARN_PERCENT` | usage percentage that raises an attention item, default 80 |

## Attention center

When a limited dimension reaches the warning percentage, the attention center shows an item for it, and a critical one at 100 percent. Items are visible to instance admins and link to the team or project.

## Limits of this feature

- Deploys are not blocked. An app that already exists keeps deploying even if its scope is over quota.
- Memory and CPU count configured reservations, not live container usage.
- Preview environments use the platform preview caps as well; the lower of the two applies.
