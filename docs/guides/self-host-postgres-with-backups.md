---
title: Self-host Postgres with verified backups
description: "Run a managed Postgres on your own server, back it up to S3-compatible storage on a schedule, and restore it safely into a new database to prove the backup works."
---

# Self-host Postgres with verified backups

A backup you have never restored is a hope. This guide creates a Postgres database on your own server, schedules backups to an S3-compatible bucket, and rehearses a restore without touching live data. It assumes a running instance and a configured `levelrail-cli`; see [getting started](/getting-started).

## 1. Create the database

```bash
levelrail-cli databases create --name main --engine postgres --version 16
```

Other engines work the same way (`redis`, `mysql`, `mongodb`, `mariadb`, `keydb`, `dragonfly`, `clickhouse`). To connect an app to it, see [connecting apps to databases](/connecting-apps-to-databases).

## 2. Connect a backup target

A backup target is a separate resource that any number of databases and volumes can use. Credentials are write-only: they go in at creation and are never returned.

```bash
levelrail-cli backup-targets create \
  --name primary-r2 \
  --provider r2 \
  --endpoint YOUR_ENDPOINT \
  --bucket levelrail-backups \
  --access-key-id YOUR_KEY_ID \
  --secret-access-key YOUR_SECRET
```

`--provider` accepts `aws`, `r2`, or `custom`. `--endpoint` is required for `r2` and `custom`. Then test it:

```bash
levelrail-cli backup-targets test TARGET_ID
```

The test calls `HeadBucket`. It proves the bucket is reachable with those credentials, not that the key can write or delete. The first scheduled backup is the real proof.

## 3. Schedule backups

```bash
levelrail-cli backups schedule set main \
  --target TARGET_ID \
  --cron "0 3 * * *" \
  --retain 7 \
  --retain-days 30
```

The cron expression is validated when you submit it. Retention has two dimensions that combine: keep at most the newest `--retain` successful backups, and never keep any older than `--retain-days`. A backup is deleted when either rule would remove it. Zero turns a dimension off.

App volumes use the same target and retention model:

```bash
levelrail-cli app-volume-backups schedule set my-app uploads --target TARGET_ID --cron "0 4 * * *" --retain 14
```

## 4. Check that backups verify themselves

Every scheduled backup that succeeds is downloaded and re-hashed afterwards, and the history shows whether it passed. Manual backups are not verified automatically. Trigger one by hand and read the history:

```bash
levelrail-cli backups trigger main
levelrail-cli backups list main
```

## 5. Rehearse a restore

Restoring as new builds a separate database from a backup and never touches the original:

```bash
levelrail-cli backups restore-as-new main --backup BACKUP_ID --new-name main-rehearsal
```

Connect to `main-rehearsal`, run a query you can check, then delete the copy. Do this on a calendar, not once.

Restoring in place overwrites the live database, is destructive, and needs you to type the database name:

```bash
levelrail-cli backups restore main --backup BACKUP_ID --confirm main
```

## 6. Get told when backups stop

A `backup_missing` alert rule fires when the newest successful backup trails the schedule by more than the rule's grace period (default 6 hours). It catches both a schedule that stopped running and repeated failures. Route it to a channel that does not depend on the server being backed up. See [observability](/observability#alert-rules).

## What this does not cover

- Point-in-time restore for Postgres is separate (`levelrail-cli pitr`) and only recovers moments after you enable it. See [managing databases](/managing-databases).
- The control plane's own database needs its own backup, plus an off-server copy of the master key. See [control plane backup](/control-plane-backup) and [disaster recovery](/disaster-recovery).
- Bind mounts are not backed up, only Docker-managed named volumes.

## Next steps

- [Back up Postgres to S3 tutorial](/tutorials/back-up-postgres-to-s3)
- [Backups and storage](/backups-and-storage)
- [Managing databases](/managing-databases)
