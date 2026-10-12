---
title: "Teams and roles"
description: Teams own projects, environments, apps and databases. Four roles (owner, admin, developer, viewer), invitations, team scoped API tokens, protected environments and the dashboard and CLI switchers.
---

A team owns projects. Each project owns its environments, and every app and database filed under a project belongs to that project's team. Two teams on one instance cannot see or change each other's apps, databases, domains, secrets, logs, deploys, tokens or team audit trail.
## The model

| Object | Belongs to |
| --- | --- |
| Team | the instance |
| Project | one team |
| Environment | one project (shared Dev, Test, UAT and Production environments belong to the instance) |
| App, database | one project, so one team |

An app or database with no project belongs to the **default team**. Upgrading an existing install creates the default team, files every existing project under it, and adds every existing user to it, so nothing changes until you create a second team.

The default team is the instance itself. Its members keep instance wide pages (nodes, settings, templates) under their own abilities, exactly as before. Instance admins (users with the `root` ability) see every team.

## Roles

There are four roles. Each maps onto the same abilities API tokens and IAM policies use, so one engine decides every request.

| Role | Can |
| --- | --- |
| Owner | everything an admin can, plus rename or delete the team and transfer ownership |
| Admin | deploy anywhere, including protected environments; manage members and invitations |
| Developer | deploy, edit env vars and secrets, read logs; not in protected environments |
| Viewer | read apps, databases, logs and deploys; change nothing |

Admins can invite and manage developers and viewers. Only owners assign or remove admins and owners. A team always keeps at least one owner: removing, demoting or leaving as the last owner is refused until ownership is transferred.

In the default team, a member's abilities are still set on the Users page; changing their team role there also sets their abilities to that role's.

## Protected environments

Mark an environment protected (Settings, Environments, or `levelrail-cli apps environments update <id> --protected=true`). In a protected environment a developer cannot deploy, restart, roll back or change env vars; an admin or owner can. Deploys into a protected environment can also wait for approval: an admin or owner approves them from **Approvals**.

## Invitations

Invite by email or by link. A link invite has no email and works once, for whoever opens it first. Every invite expires: the default is 7 days, `APP_TEAM_INVITE_TTL_HOURS` sets the instance maximum, and a shorter expiry can be set per invite.

Someone who is signed in joins with their account. Anyone else creates an account that belongs to the invite's team only. An invite is spent before the account is created, so a replayed or raced link can never add a second person.

## Dashboard

- **Settings, Teams** lists your teams, members, roles, invitations and the team audit trail.
- The header switcher narrows every list to one team and, optionally, one project. It only ever narrows: you never see more than your roles allow.
- Your role in the selected team shows as a badge next to the switcher.
- A page you cannot open shows why, with a link to Teams and back to the dashboard.

## CLI

```bash
levelrail-cli teams list
levelrail-cli teams create "Payments"                 # instance admins
levelrail-cli teams invites create org_xxx --role developer --email dev@example.com
levelrail-cli teams invites create org_xxx --role viewer        # link invite
levelrail-cli teams invites accept <token>
levelrail-cli members list org_xxx
levelrail-cli members set-role org_xxx user_xxx admin
levelrail-cli members remove org_xxx user_xxx
levelrail-cli members leave org_xxx
levelrail-cli teams transfer org_xxx user_xxx
levelrail-cli teams roles
levelrail-cli teams events org_xxx
```

Scope any command to a team or project:

```bash
levelrail-cli --team org_xxx apps list
levelrail-cli --project proj_xxx --team org_xxx apps list
levelrail-cli teams use org_xxx --project proj_xxx   # saved per profile
levelrail-cli teams use                              # clear
```

`APP_TEAM` and `APP_PROJECT` do the same from the environment. Precedence is flag, then environment, then the saved context. `--project` before the command name scopes the request; after it, a command's own `--project` flag keeps its meaning.

## Team scoped tokens

A token can be limited to one team, optionally one project, and a role no higher than yours:

```bash
levelrail-cli tokens create --name ci --abilities read,deploy \
  --scope-team org_xxx --scope-project proj_xxx --scope-role developer
```

A scoped token sees only that team (or project), cannot reach instance wide routes, and its abilities cannot exceed the role. The same fields are on `POST /api/v1/auth/tokens` as `team_id`, `project_id` and `team_role`.

## API

| Route | Who |
| --- | --- |
| `GET /api/v1/teams` | any member; lists the caller's teams |
| `POST /api/v1/teams` | instance admins |
| `GET, PATCH, DELETE /api/v1/teams/{id}` | members; rename and delete need owner |
| `GET /api/v1/teams/{id}/members` | members |
| `PUT, DELETE /api/v1/teams/{id}/members/{user}` | admins and owners |
| `DELETE /api/v1/teams/{id}/members/me` | the member leaving |
| `POST /api/v1/teams/{id}/transfer` | owners |
| `GET, POST /api/v1/teams/{id}/invites`, `DELETE .../invites/{invite}` | admins and owners |
| `POST /api/v1/team-invites/accept` | the invite token holder |
| `GET /api/v1/teams/{id}/events` | admins and owners |

The `X-Scope-Team` and `X-Scope-Project` headers (or the dashboard's `scope_team` and `scope_project` cookies) narrow any list.

## How isolation is enforced

- Every `/apps/{name}` and `/databases/{name}` route resolves the resource's team before its handler runs. Another team's resource answers 404, so its existence does not leak.
- Every `/projects/{id}`, `/environments/{id}`, `/organizations/{id}`, `/teams/{id}` and `/deploy-approvals/{id}` route checks the owning team the same way.
- Lists (apps, databases, deploys, approvals, domains, usage, traffic, attention, projects, environments and their counts) filter by the same rule.
- A member of only non-default teams gets 403 on every instance wide route not reviewed for teams. New routes start closed.
- Creating or moving an app or database into a project checks your role in that project's team; a non-default team member must name a project.
- A generated test drives every route as every role against another team and fails on anything but 403 or 404.

## What teams do not cover yet

- Template deploys, imports and other instance wide create flows stay with instance members; team members create apps and databases directly.
- Instance notification channels and web push stay instance wide; team members cannot enroll in web push.
