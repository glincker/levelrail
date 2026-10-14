---
title: "Secret rotation policies"
description: Give an app secret a rotation schedule, let the control plane generate and apply new values, and see what is due or stuck.
---

A rotation policy puts a schedule on one app secret: how often it should change, who owns it, and whether the control plane may generate the new value itself. This is separate from [master key rotation](master-key-rotation.md), which re-wraps the keys that protect every secret.

## What a policy holds

| Field | Meaning |
| --- | --- |
| Interval | Days between rotations. The clock starts from the later of the last policy rotation and the last time the value was written, so editing the value by hand also resets it. |
| Grace window | Hours the previous value stays available as `<KEY>_PREVIOUS` in the app's environment, so a consumer that still holds the old value is not cut off at once. `0` keeps nothing. |
| Owner | Who is accountable for this secret. Defaults to the person who created the policy. |
| Auto-rotate | Lets the control plane generate a random 64 character value and apply it. Leave it off for credentials issued by a third party. |

A policy never stores a value. Values are never logged and never returned by any rotation endpoint.

## Status

Each policy reports a status computed from the next due date: `ok`, `due_soon` (inside `APP_SECRET_ROTATION_DUE_SOON_DAYS`), `overdue`, or `unknown`. Overdue policies and rotations that need attention appear in the attention center.

## What a rotation does

1. Checks the secret exists and is not locked. A locked secret is never rotated; unlock it first.
2. Stores the old value as `<KEY>_PREVIOUS` when a grace window is set.
3. Records progress, writes the new value, and writes an audit entry.
4. Restarts the app through the normal health-gated cutover. A stopped app is not started, it picks the value up when it next starts.

When the grace window ends, `<KEY>_PREVIOUS` is deleted and that is audited too.

## When a rotation half succeeds

If the new value is stored but the restart fails, the policy moves to `restart_failed` and shows in the attention center as an incident. The control plane retries only the restart on every pass and never generates a second value. You can also press **Rotate now** (or run `apps secrets rotation rotate`) to retry it immediately. If the value could not be written at all, nothing was changed and the policy moves to `failed` with a plain reason. A failed policy is not retried automatically: fix the cause, then rotate by hand.

## Dashboard

Open an app's **Environment** page and use the **Rotation** section. It lists each secret with its policy, status and next due date, and **Rotate now** asks for confirmation before replacing the value and restarting the app.

## CLI

```sh
levelrail-cli apps secrets rotation set web JWT_SECRET --interval-days 30 --grace-hours 24 --auto
levelrail-cli apps secrets rotation list web
levelrail-cli apps secrets rotation due
levelrail-cli apps secrets rotation rotate web JWT_SECRET --yes
levelrail-cli apps secrets rotation remove web JWT_SECRET
```

`due` lists policies across every app and needs the root ability.

## API

| Route | Purpose |
| --- | --- |
| `GET /api/v1/apps/{name}/secret-rotation` | Policies of one app. |
| `PUT /api/v1/apps/{name}/secrets/{key}/rotation` | Create or update a policy. |
| `DELETE /api/v1/apps/{name}/secrets/{key}/rotation` | Remove a policy. |
| `POST /api/v1/apps/{name}/secrets/{key}/rotation/rotate` | Rotate now. Answers `409` for a non-auto policy or a refused rotation, `502` when the restart failed. |
| `GET /api/v1/secret-rotation` | Every policy, root only. |

## Limits

- Only per-app secrets are covered. Shared variables at project, organization and environment level are not rotated by policy.
- Database role passwords keep their own [rotation path](database-access.md). A policy does not rotate them, because nothing would receive the new password.
- Generated values are random hex strings. A secret that must follow a specific format should not be auto-rotated.
- Apps that read `<KEY>_PREVIOUS` get a grace window. Apps that do not simply move to the new value at the restart.
