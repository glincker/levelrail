---
description: Filter, tag, bulk-act on, clone and promote apps when you run dozens of them.
---

# Managing apps at scale

This page covers the tools for running many apps: filtering the list, environments and tags, bulk actions, cloning an app, and promoting a release from staging to production. Every operation is available in the dashboard, the CLI, and the API. Bulk actions and promotion are also MCP tools.

## Environments and tags

An **environment** (production, staging, or any name you create) belongs to a project, and an app is placed in one with `PUT /api/v1/apps/{name}/environment` or the bulk `set-environment` action. An environment can be marked **protected**, which makes deploys and promotions into it require confirmation and approval. See [Projects, organizations, and environments](./projects-and-organizations.md).

**Tags** are free-form `key` or `key:value` labels that cut across projects. See [Tags](./tags.md) for the naming rules.

## The apps list

The dashboard list at `/apps` has a status strip (running, deploying, failing, stopped), a search box, environment chips, a tag filter, and a checkbox on every row. The list is virtualized, so hundreds of apps stay responsive.

**Saved views** store the current search, environments and tags under a name. They are kept in your browser's local storage, not on the server: the control plane has no per-user preference store, and a view is a convenience rather than shared state.

Over the API:

```bash
curl "https://control-plane/api/v1/apps?environment=staging&tag=team:core&q=web&limit=50&offset=0" \
  -H "Authorization: Bearer $TOKEN"
curl "https://control-plane/api/v1/apps-summary?environment=production" \
  -H "Authorization: Bearer $TOKEN"
```

Apps you may not read (an IAM Deny on `app:<name>`) are left out of both.

## Bulk actions

Select apps in the list and use the bar that appears at the bottom, or from the CLI:

```bash
levelrail apps bulk restart --tag tier:edge --dry-run
levelrail apps bulk add-tag team:core --names web,api --yes
levelrail apps bulk set-environment staging --env development
levelrail apps bulk delete --tag scratch
```

Actions: `redeploy`, `restart`, `stop`, `start`, `add-tag`, `remove-tag`, `set-environment`, `move-to-project`, `delete`.

`POST /api/v1/apps/bulk` takes `action`, one of `names`, `tag` or `environment`, an optional `value`, and `dry_run`. It always answers **207** with one result per app:

| Status | Meaning |
| --- | --- |
| `ok` | applied |
| `would_apply` | dry run, nothing changed |
| `denied` | your permissions do not allow this action on this app |
| `not_found` | no such app |
| `skipped` | not applicable (for example a redeploy into a protected environment, or a tag cap reached) |
| `error` | the action failed for this app |

Each app is authorized on its own against your abilities and IAM policies, so a denied app never blocks the rest and is never silently dropped. Up to `APP_BULK_CONCURRENCY` (default 4) apps run at once, and a request may name at most `APP_BULK_MAX_APPS` (default 200).

`delete` needs `confirm_names` equal to the exact set of target apps. The dashboard and CLI run a dry run first and fill it from the list you confirmed. Every deleted app gets its own audit log entry. `redeploy` skips apps in protected environments, so approval flows are never bypassed in bulk.

The MCP server exposes `bulk_apps` (mutating) and `bulk_delete_apps` (destructive, gated like other destructive tools).

## Clone an app

```bash
levelrail apps clone web web-staging --preview
levelrail apps clone web web-staging --domain-suffix stg --copy-secrets --environment env_abc
```

A clone copies the image, port, command, plain env, secret names, vault references, replicas, strategy, resources, health checks, hooks, egress policy, volume definitions, and project.

- **Domains** are never copied as is, because two apps must not answer the same host. By default the clone has none. `--domain-suffix stg` derives `web-stg.example.com` from `web.example.com`.
- **Secret values** are copied only with `--copy-secrets`, and only if you hold `read:sensitive`. Each value is decrypted and encrypted again under the new app's own key and slot binding.
- **Volumes** are recreated as new, empty volumes. Copying volume data (`--with-data`) is not supported.
- Scheduled tasks, node placement and deploy history are not copied.

## Promote a release

```bash
levelrail apps promote web-staging --to env_prod --preview
levelrail apps promote web-staging --to env_prod --include-env --confirm
```

The preview shows a diff before anything changes: image, replicas, resources, health, and the **names** (never values) of env keys that would be added, removed, or differ. Domains, ports, node placement, volumes and secret values are never touched. Applying promotes the source's deployed image reference to the destination and starts a deploy through the normal deploy path. With `--include-env`, env keys the source added or dropped are applied too; keys both apps have keep the destination's own value.

Guardrails:

- Promoting into an environment named `production`, or any protected one, needs `--confirm`. Protected environments also go through deploy approval.
- A source that is unhealthy or whose last deploy failed is refused. `--force` overrides.
- Both apps get an audit log entry.
