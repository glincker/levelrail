---
description: Prove a backup restores before you need it. Run restore drills, read backup health per app, restore a volume into a new app or node, and respond when a drill fails.
---

# Prove your backups work

A backup file is not a backup until a restore of it has worked. Levelrail separates the two: **last backup** says an object was uploaded, **last verified restore** says a copy of it was restored into a scratch resource and checked. This guide shows how to get the second number green and what to do when it is not.

For targets, schedules and retention basics, see [Backups and storage](backups-and-storage.md).

<InlineToc default-open />

## What a drill does

A restore drill takes a stored backup and:

1. Checks the object still exists and has the size that was uploaded (detects a deleted or truncated object without downloading it).
2. Downloads it and compares its SHA-256 with the checksum recorded at backup time.
3. Restores it into a scratch resource that is not attached to any app.
4. Validates the result.
5. Destroys the scratch resource, whether the drill passed or failed.

| Resource | Scratch resource | Validation |
| --- | --- | --- |
| App volume | A new Docker volume named `drill-<id>` | The restored tree is read back and compared with the archive: file count, bytes, paths, modes, owners, symlink targets and content hashes. It is also compared with the manifest recorded at backup time. |
| Postgres, MySQL, MariaDB | A throwaway container from the database's own image | The dump is replayed with the normal restore command, then tables are counted. A dump with a failing statement fails the drill. |
| Other engines | None | Object check, checksum and the engine's dump trailer or header check. This proves the object is whole, not that it replays. |
| SQLite volume snapshot | None | Checksum and the SQLite file header. |

Drills run on the control plane's own Docker daemon. A volume that lives on another node is restored for the drill from the object store, so the drill still proves the backup itself, not that node.

## Run one

Dashboard: open **Backups** and press **Prove restore** on a row. CLI:

```bash
levelrail-cli backups drill run --backup bkh_abc123
levelrail-cli backups drill list --app web --volume data
levelrail-cli backups drill show bkd_xyz789
```

`drill run` waits for the result and exits non-zero when the drill fails, so it works in a script or a cron job.

## Automatic drills

Every resource that has a succeeded backup is drilled on its newest backup once per `APP_BACKUP_DRILL_INTERVAL_HOURS` (default `168`, one week; `0` turns scheduled drills off). The result history is on the **Backups** page and in `levelrail-cli backups drill list`.

A failed drill fires the platform-wide `restore_drill_failed` alert rule. Create it under **Alerts** like any other rule; it resolves when the next drill of that resource passes. A resource also shows **Failing** on the Backups page until then.

## Read backup health

The **Backups** page groups every backed up database and app volume by app:

| Column | Meaning |
| --- | --- |
| State | Healthy, Needs attention, Failing, Not verified, or No backup. The reason is printed under the badge. |
| Last backup | When the newest succeeded backup started. |
| Last verified restore | When a drill last passed. "Never verified" means you only know a file exists. |
| Size | Total stored size of the resource's succeeded backups. |
| Next run | The next scheduled backup, from the cron schedule. |

Terminal view: `levelrail-cli backups health`.

## Restore

Restores never overwrite by default. Pick **Restore** on a volume row for the wizard, or:

```bash
# A new volume next to the original, on the same node
levelrail-cli backups volumes restore web data --backup bkh_abc123

# A new volume that the app "web-staging" will mount when it deploys
levelrail-cli backups volumes restore web data --backup bkh_abc123 --target-app web-staging

# Onto another node
levelrail-cli backups volumes restore web data --backup bkh_abc123 --node node_2
```

The restore refuses a volume name that already exists on the node. For an in-place restore over the live volume, use `app-volume-backups restore`, which asks you to type the volume name.

Encrypted backups are decrypted with this control plane's key. To restore somewhere with no control plane, decrypt by hand:

```bash
age -d -i backup-encryption.key object.tar.zst.age | zstd -d | tar -x -C ./restore
```

For databases, the same Backups row links to the database page, where **Restore as new database** and the point-in-time restore picker live. Point-in-time restore needs PITR enabled on the database first; see [Managing databases](managing-databases.md).

## Encryption and the key

New volume backups are compressed with zstd and encrypted with [age](https://age-encryption.org) on the control plane before upload, so the bucket never sees plaintext. The key is generated on first use and written to `backup-encryption.key` in `APP_DATA_DIR` (override the path with `APP_BACKUP_ENCRYPTION_KEY_FILE`).

::: warning
Copy that file somewhere that is not this server. Lose it and encrypted backups cannot be restored, including after a disaster that destroys the control plane.
:::

Set `APP_BACKUP_ENCRYPTION=off` to keep compression but skip encryption. Older backups taken before this feature stay readable; each backup records how it was encoded.

## Consistency hooks

A running database's files are only consistent if it is quiet. Per volume you can set, from the **Policy** button or `levelrail-cli backups volumes policy set`:

- **Command before backup**: runs inside the app's container. A non-zero exit fails the backup, so an inconsistent volume is never archived.
- **Command after backup**: runs afterwards, even if the backup failed.
- **Pause the app**: freezes the container while the archive streams. It resumes when the upload ends.

## Retention

Besides the schedule's keep-last and age limits, a volume can keep the newest backup of each of the last N days, weeks and months:

```bash
levelrail-cli backups volumes policy set web data --retain-daily 7 --retain-weekly 4 --retain-monthly 6
```

Buckets count periods that actually have a backup, so a week with no backups does not shrink what is kept. The newest backup is never removed by retention. Pruning runs after each successful scheduled backup.

## Budgets

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_BACKUP_MAX_SIZE_MB` | unset | A volume archive larger than this fails the backup and is not retried. |
| `APP_BACKUP_TIMEOUT_MINUTES` | unset | One backup, archive plus upload, is cancelled after this long. |
| `APP_BACKUP_UPLOAD_ATTEMPTS` | `1` | How many times a failed upload is retried from a fresh archive. |
| `APP_BACKUP_DRILL_INTERVAL_HOURS` | `168` | Scheduled drill period per resource. `0` disables. |
| `APP_BACKUP_STALE_RUNNING_MINUTES` | `360` | A backup still "running" after this long is settled from the bucket. |

A live volume cannot be re-read from an offset, so a half-uploaded object is aborted and the volume is archived again rather than resumed. If the control plane stops mid-backup, a sweep finishes the row: an object that reached the bucket is checksummed and recorded as succeeded, a missing one is marked failed as interrupted.

## Bucket protection

**Check buckets** (or `levelrail-cli backups protection --refresh`) probes each backup target for object lock and versioning, and writes and deletes one small canary object to learn whether the key can delete. The result is also refreshed daily.

| Level | Meaning |
| --- | --- |
| Locked | Object lock is on. A leaked key cannot erase backups inside the retention window. |
| Versioned | Versioning is on but not object lock. Old versions survive an overwrite. |
| Open | Neither. A leaked key or a bad script can delete or overwrite every backup. |

Retention needs delete permission, so the key must be able to delete; object lock is what makes that safe. Backup object names include the backup ID, so a new backup never overwrites an old one.

## What was tested

These ran against real Docker volumes and a real S3 compatible server (SeaweedFS), no fakes:

| Check | Result |
| --- | --- |
| Back up a volume with a uid 1234:5678 directory (750), a 640 file with a 2020 mtime and a symlink, delete the volume, restore into a new volume | Content hashes, owners, modes, mtime and symlink target identical; manifest digest identical |
| Drill a healthy backup | Passes, scratch volume removed |
| Flip one byte of the stored object | Drill fails at restore, scratch volume removed |
| Truncate the stored object | Drill fails at the object check |
| Restore a truncated object over a live volume | Refused, the live volume is untouched |
| Pre-backup hook, post-backup hook and pause | Hooks ran inside the container, container not left paused, a failing pre-hook fails the backup |
| Postgres dump drilled into a scratch Postgres | Restores, two tables counted; a modified dump fails |

Limits: drills for engines other than Postgres, MySQL and MariaDB check the object, not a replay. Object lock and versioning detection depends on the provider answering those API calls; a provider that does not shows as Open.
