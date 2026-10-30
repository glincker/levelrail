---
title: "Pre-deploy database snapshots"
description: "Snapshot an app's linked databases before each deploy and roll back the code together with the data."
---

When an app is linked to a managed database, a deploy can run a migration that a plain code rollback cannot undo. Turn on pre-deploy snapshots and the control plane takes a backup of each linked database right before the deploy, records it on the deploy, and lets you roll back with the data.

## Turn it on

```bash
levelrail-cli apps settings predeploy-snapshot my-app --mode on --target <backup-target-id>
```

Modes:

- `off`: no snapshot (default).
- `on`: snapshot first. If the snapshot fails the deploy still goes ahead and the failure is recorded.
- `require`: a failed snapshot blocks the deploy.

Without `--target` the only configured backup target is used. The same setting is on the app page in the dashboard and at `PUT /api/v1/apps/{name}/predeploy-snapshot`.

## Roll back with data

```bash
levelrail-cli apps rollback my-app --with-data
```

This restores the snapshot of the newest deploy into a new database (named `<db>-rb-<suffix>`), points the app's database links at it, and redeploys the previous release. The original database is not modified, so you can still inspect or reattach it. Pass `--deploy <id>` to undo a specific deploy. Apps in a protected environment must use the plain rollback.

In the dashboard, open the menu on a deploy in the app's Deploys history and choose Roll back with data. The dialog shows the snapshots taken before that deploy and warns that databases are restored into new ones and the links swapped. It is disabled, with the reason, when no usable snapshot exists or the environment is protected.

Snapshots are listed at `GET /api/v1/apps/{name}/predeploy-snapshots`.

## Limits

Set on the control plane:

- `APP_PREDEPLOY_SNAPSHOT_TIMEOUT`: how long a snapshot may run (default `2m`).
- `APP_PREDEPLOY_SNAPSHOT_MAX_SIZE_MB`: a larger snapshot is discarded and counts as failed (default 2048).
- `APP_PREDEPLOY_SNAPSHOT_KEEP`: snapshots kept per database (default 3).

Rollbacks, queued offline deploys and approval-gated deploys do not take a snapshot.
