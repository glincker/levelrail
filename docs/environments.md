---
description: Instance-wide environments (development, test, UAT, production and custom), the dashboard environment switcher, and moving apps and databases between them with approval for protected ones.
---

# Environments

An environment labels where an app or database runs: development, test, UAT, production, or one you define. Environments are instance-wide, so the same four are available to every project. Tagging resources with an environment lets you filter the dashboard, protect production, and write access rules per environment.

<InlineToc default-open />

Creating and renaming environments, the switcher and the database move route are behind the `global-environments` experimental flag: set `APP_EXPERIMENTAL=global-environments` on the control plane (see [Experimental features](experimental-features.md)). Existing per-project environments keep working with or without it.

## Built-in environments

Four environments exist on every instance and cannot be deleted.

| Name | Kind | Protected |
| --- | --- | --- |
| Development | `dev` | No |
| Test | `test` | No |
| UAT | `uat` | No |
| Production | `production` | Yes |

## Kinds

Every environment has a kind: `dev`, `test`, `uat`, `production`, `preview` or `custom`. Kinds are what [IAM policies](access-control.md#iam-policies) and [AI control](ai-control.md) match on, so a rule like "deny deploys on `environment-kind:production`" covers every production-kind environment, whatever its name.

`preview` is reserved for the environments Levelrail creates for pull request previews. You cannot create one, and you cannot change an environment to or from that kind.

## Create and manage

```bash
levelrail-cli environments list
levelrail-cli environments create --name Staging --kind custom --sort-order 25
levelrail-cli environments update env_abc --name "Pre-prod" --kind uat --protected
levelrail-cli environments update env_abc --protected=false
levelrail-cli environments delete env_abc --move-to env_dev
```

In the dashboard, open Settings, Environments. The list shows each environment's kind, whether it is protected, and how many apps and databases it holds.

Deleting:

- A built-in environment cannot be deleted (403).
- An environment that still has apps or databases cannot be deleted (409) unless you pass `--move-to`, which moves them to another environment first.
- A bulk move with `--move-to` is refused if the source or the destination is protected. Move those one at a time so each gets its approval.

## Move an app or database

```bash
levelrail-cli apps move-env myapp env_dev
levelrail-cli databases move-env mydb env_dev
```

Pass an empty id (`""`) to remove the tag. In the dashboard, use the Change button next to the environment on the app's Overview.

A move **into or out of a protected environment** works differently:

1. Without `--confirm` the request is refused with 409 and nothing changes.
2. With `--confirm` it becomes a pending approval instead of moving right away.
3. A different person with the `deploy` ability approves it (Deploy approvals, or `levelrail-cli deploy-approvals`). The person who asked cannot approve their own request.
4. Only then does the app or database move.

```bash
levelrail-cli apps move-env myapp env_production --confirm
```

A move during a deploy freeze is refused with 423 unless you pass the freeze override. If the environment an app is leaving cannot be read, the move is refused rather than assumed unprotected.

## Protection

Mark any environment as protected to put deploys and moves behind approval:

```bash
levelrail-cli environments update env_abc --protected
```

Turning protection on or off works without the experimental flag. Deploys to a protected environment already needed confirmation and approval before this release, and that is unchanged.

## Switcher and filters

With the flag on, the dashboard header has an environment switcher. Choosing an environment filters the apps, databases, deployments and approvals lists, and the choice is remembered in your browser and reflected in the URL as `?environment=`.

The same filter works on the API and CLI. It accepts an environment id or name:

```bash
curl -H "Authorization: Bearer $TOKEN" "https://<control-plane>/api/v1/apps?environment=production"
levelrail-cli apps list --environment production
levelrail-cli databases list --environment production
```

The approvals list filters by environment id only.

## Environment variables

Per-environment variables and secrets work for these environments the same way they do for project environments. See [Projects and organizations](projects-and-organizations.md) if you use per-project environments too.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/environments` | `read` |
| `POST` | `/api/v1/environments` | `write` |
| `PATCH` | `/api/v1/environments/{id}` | `write` |
| `DELETE` | `/api/v1/environments/{id}` | `write` |
| `PUT` | `/api/v1/apps/{name}/environment` | `write` |
| `PUT` | `/api/v1/databases/{name}/environment` | `write` |

Create takes `{"name", "kind", "protected", "sort_order"}`. A move takes `{"environment_id", "confirm", "override_freeze", "override_reason"}` and answers 409 (needs confirmation), 202 with a pending approval, 423 (freeze) or 200.

## Next steps

<CardGroup :cols="2">
<Card title="Access control" href="/access-control">

Roles, guest visibility and policies per environment.

</Card>
<Card title="AI control" href="/ai-control">

Limit agents to chosen environment kinds.

</Card>
</CardGroup>
