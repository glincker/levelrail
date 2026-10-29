---
title: "Backup copies"
description: Copy every new backup to a second destination after it lands in the first, track how far behind the copy is, and get alerted when it falls behind.
"og:url": "https://docs.levelrail.com/backup-copies"
---

A backup that lives in one bucket is lost with that bucket. Backup copies mirror each finished backup to a second destination, so a bucket or provider problem does not take your only restore point with it.

## Turn it on

Connect two backup destinations first (see [Backups and storage](/backups-and-storage)). Then choose the second one:

- Dashboard: Settings, Backup targets, Second destination.
- CLI: `levelrail-cli backups destinations copy set <destination>`. Use `off` to stop, `show` to read the setting.
- API: `PUT /api/v1/backups/copy-settings` with `{"enabled": true, "target_id": "..."}`.

Only backups that finish after you turn copying on are copied. Objects are copied byte for byte under the same key, so encrypted dumps stay encrypted. Credentials come from the same secret store as the destinations themselves and are never logged.

## See the state

`levelrail-cli backups copies list` (or `GET /api/v1/backups/copies`, or the table on the same settings page) shows each backup with its copy status and lag. Lag is the time from the backup finishing to the copy landing, or to now while it is still waiting. A failed copy is retried on the next pass with its last error shown.

## Get alerted

Create a rule of kind `backup_copy_behind`; it also ships as a default rule. It fires while any backup has had no copy for longer than the limit:

```
levelrail-cli apps alerts create <app> --kind backup_copy_behind --name "backup copy behind"
```

Set a per-rule limit with `--for-duration`, for example `2h`.

## Settings

Control plane environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BACKUP_COPY_LAG_ALERT` | `1h` | Default time a backup may go without a copy before `backup_copy_behind` fires. Duration. |
| `APP_BACKUP_COPY_CONCURRENCY` | `2` | How many copies run at once. Positive integer. |

Restore always reads from the primary destination; the second copy is for recovery when the primary is gone.

## Verification and manifest

Each copy is read back and its SHA-256 compared with the primary's recorded checksum. A `<object>.manifest.json` sidecar (checksum, codec, tree digest, size) is written next to the copied object, so a restore from the second destination alone has what it needs. A copy only counts as succeeded after both are in place.

## Local directory targets

For small boxes and tests, set `APP_BACKUP_LOCAL_ROOT` to an absolute directory, then add a target with `levelrail-cli backups targets add --type local --name NAME --path SUBDIR`. The path is relative to the root and cannot leave it (symlinks included). No keys are needed. The root is returned read-only as `local_root` by `backups targets list` and `show`, and the dashboard form shows it as the folder prefix.
