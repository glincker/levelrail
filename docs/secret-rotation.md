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

## Troubleshooting

### Rotation status shows "overdue" but was never set to rotate

Likely cause: The rotation policy is set to a very short interval (for example, 1 day), and the last rotation was more than that interval ago.

How to confirm: Check the policy details: `levelrail-cli apps secrets rotation list <app>` shows the interval and next due date. Compare the next due date to today.

Fix: Rotate the secret manually: `levelrail-cli apps secrets rotation rotate <app> <key> --yes`. Then adjust the interval if needed: `levelrail-cli apps secrets rotation set <app> <key> --interval-days 30`.

Still stuck: Check when the secret was last written: `levelrail-cli audit-log --resource <app> | grep <key> | tail -5`.

### Rotation fails with "restart_failed"

Likely cause: The app failed to come up healthy after the new secret was applied.

How to confirm: Check the app status: `levelrail-cli apps status <app>` shows the current state. Check recent logs: `levelrail-cli apps logs <app> --since 5m` for errors related to the secret.

Fix: The new secret value has already been applied. Verify the secret is correct in your app configuration, then retry the restart: `levelrail-cli apps secrets rotation rotate <app> <key> --yes`. Or manually inspect and correct the issue, then restart the app: `levelrail-cli apps restart <app>`.

Still stuck: Check if the app is expecting a specific secret format or has the secret hardcoded somewhere (which would break after rotation).

### Grace window value (`<KEY>_PREVIOUS`) not available in the app

Likely cause: The grace window is set to 0 (disabled), or the restart happened before the app could read the previous value.

How to confirm: Check the policy: `levelrail-cli apps secrets rotation list <app>` shows the grace window duration. If it is 0, the previous value is not kept. Check how long the grace window is: after a rotation, `<KEY>_PREVIOUS` should be available for that many hours.

Fix: To enable the grace window, set it: `levelrail-cli apps secrets rotation set <app> <key> --grace-hours 24`. For the next rotation, the previous value will be available. To use the previous value now, you must manually set it back (since rotation has already happened).

Still stuck: See [What a rotation does](/secret-rotation#what-a-rotation-does) for the grace window behavior.

### Auto-rotate generated a value in wrong format

Likely cause: The auto-generated random hex string does not match the format the third-party service expects.

How to confirm: Check what the app is expecting (check the docs or the service provider). Auto-generated values are 64 random hex characters. If the service expects something different (for example, a URL-safe base64 string or a specific format), auto-rotate will not work.

Fix: Turn off auto-rotate for this secret: first remove the current policy (`levelrail-cli apps secrets rotation remove <app> <key>`), then recreate it without `--auto`: `levelrail-cli apps secrets rotation set <app> <key> --interval-days 30`. Manual rotation allows you to generate the value in the correct format from the third-party service and provide it.

Still stuck: See [Limits](/secret-rotation#limits) for details on auto-generated value format.

### Secret is locked and cannot be rotated

Likely cause: Another operation is currently writing to this secret (for example, a deploy in progress).

How to confirm: Check the secret status: `levelrail-cli apps secrets get <app> <key> --json | jq .locked` should return true if locked. Wait for any in-flight deploys to complete: `levelrail-cli apps status <app>` should show no pending state.

Fix: Wait for the lock to clear (usually takes 1-2 minutes as the current operation finishes). Then retry the rotation: `levelrail-cli apps secrets rotation rotate <app> <key> --yes`. If the lock persists after 10 minutes, restart the control plane or check logs for errors.

Still stuck: Check control plane logs for secret lock errors: `levelrail logs --since 30m | grep "secret.*lock"`.

### Policy removed but `<KEY>_PREVIOUS` still exists

Likely cause: The grace window had not expired yet when the policy was deleted, so the previous value is still present.

How to confirm: Check if the env variable still exists: `levelrail-cli apps env <app> | grep PREVIOUS`. If it is there, wait for the grace window to expire and the app to be restarted, or manually delete it.

Fix: The `<KEY>_PREVIOUS` will be automatically deleted when the grace window expires and the app is next restarted. To remove it immediately, delete the env variable manually: `levelrail-cli apps secrets unset <app> <KEY>_PREVIOUS`.

Still stuck: See [Dashboard](/secret-rotation#dashboard) or [Environment](/environment-and-secrets) to delete env variables manually.
