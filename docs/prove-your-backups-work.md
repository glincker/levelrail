---
title: "Prove your backups work"
description: Run restore drills to verify backups work before you need them.
"og:url": "https://docs.levelrail.com/prove-your-backups-work"
---

A backup file is not a backup until a restore of it has worked. Levelrail separates the two: "last backup" says an object was uploaded, and "last verified restore" says a copy of it was restored into a scratch resource and checked. This guide shows how to get the second one green and what to do when it is not.

For targets, schedules and retention basics, see [Backups and storage](/backups-and-storage).

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

To make the backup a drill needs, trigger one by hand: `levelrail-cli app-volume-backups trigger <app> <volume> --target ID`. `ID` is a storage destination from `levelrail-cli storage list`.

### Try it against a local S3 server

A destination on the same host, such as `--endpoint http://127.0.0.1:19000`, is refused with `endpoint_blocked` by default, because the control plane blocks loopback and private addresses for outbound storage calls. Set `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true` in the control plane's environment and restart it. Despite the name, this one switch also gates storage destinations, and it relaxes the same check for webhooks, so enable it only on a host you control.

The `minio/minio` image is no longer pullable from Docker Hub. Two S3-compatible servers whose Docker Hub manifests list an arm64 build (checked with `docker manifest inspect`) are `chrislusf/seaweedfs` and `dxflrs/garage`. Check the tag you pick the same way before relying on it.

## Automatic drills

Every resource that has a succeeded backup is drilled on its newest backup once per `APP_BACKUP_DRILL_INTERVAL_HOURS` (default `168`, one week; `0` turns scheduled drills off). The result history is on the **Backups** page and in `levelrail-cli backups drill list`.

A failed drill fires the platform-wide `restore_drill_failed` alert rule. Create it under **Alerts** like any other rule; it resolves when the next drill of that resource passes. A resource also shows **Failing** on the Backups page until then.

While any resource is failing, the drill history card shows a **Fix the failing restore drill** action. It re-checks the latest drill of every resource and, from the first failure's error, says which of these it is: rejected credentials (update the access key on the backup target), an unreachable destination (check the endpoint and use **Test connection**), a full host disk (use **Clean up unused Docker data**), a missing or damaged object (take a new backup), or something else (open the failed drill for its stage and error). **Re-check** clears it once the next drill passes. The same fix is listed by `levelrail-cli fixes list --subject backup`.

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

For databases, the same Backups row links to the database page, where **Restore as new database** and the point-in-time restore picker live. Point-in-time restore needs PITR enabled on the database first; see [Managing databases](/managing-databases).

## Encryption and the key

New volume backups are compressed with zstd and encrypted with [age](https://age-encryption.org) on the control plane before upload, so the bucket never sees plaintext. The key is generated on first use and written to `backup-encryption.key` in `APP_DATA_DIR` (override the path with `APP_BACKUP_ENCRYPTION_KEY_FILE`).

<Warning>
Copy that file somewhere that is not this server. Lose it and encrypted backups cannot be restored, including after a disaster that destroys the control plane.
</Warning>

Database dumps use the same key, switched on per backup target. New targets encrypt dumps by default. For an existing target, press **Enable encryption** on **Settings, Backup targets** (root only), or run `levelrail-cli backups encryption set <target-id> on`; `get` shows the current state. Only new dumps are encrypted: dumps already in the bucket stay as they are, and restores, drills and verification read both kinds. Encrypted rows show an **Encrypted** badge in the backup list. There is no key export action; the key is the `backup-encryption.key` file above.

Set `APP_BACKUP_ENCRYPTION=off` to keep compression but skip encryption. Older backups taken before this feature stay readable; each backup records how it was encoded.

## Consistency hooks

A running database's files are only consistent if it is quiet. Per volume you can set, from the **Policy** button or `levelrail-cli backups volumes policy set`:

- Command before backup: runs inside the app's container, as the container's own user, never on the host. If it exits non-zero or times out, the backup is not taken and is recorded as failed with a short reason such as `pre-backup hook failed: exit 3` or `timed out`. At the time limit the command and everything it started (its process group, when the image has `setsid`) is killed inside the container, and the after command runs only once the control plane has seen it end. If it has not ended within `APP_BACKUP_HOOK_KILL_GRACE_SECONDS` (default 5, at least 2) more, the reason says it may still be running and the after command still runs.
- Command after backup: always runs, even if the backup or the pre-command failed or timed out, so an app is never left with saving switched off.
- Pause the app: freezes the container while the archive streams. It resumes when the upload ends.

Each hook is cut off after the `backup.hook_timeout_seconds` runtime setting (default 60, seeded by `APP_BACKUP_HOOK_TIMEOUT_SECONDS`), never longer than `APP_BACKUP_HOOK_MAX_SECONDS` (default 600). At most `backup.hook_max_output_bytes` (default 4096, seeded by `APP_BACKUP_HOOK_MAX_OUTPUT_BYTES`) of a failing hook's output is kept, after masking the command text and known secret shapes; the rest is discarded. The output is never written to logs or to the backup's error, and the volume backup list returns it as `hook_output` only to callers who may change that app's hooks, because masking cannot catch a secret the command expands at run time. Both settings apply to the next hook without a restart. Hook errors never include the command, because hooks often carry credentials. No hooks and no schedule are enabled by default.

Setting or clearing a hook needs the `write:sensitive` ability on that app, because a hook is code that runs inside your container. Team roles, protected environments and IAM policies apply as they do for the change itself; callers who cannot change the hooks see `hooks_hidden: true` instead of the commands.

Only one backup of a volume runs at a time, so one run's after command cannot resume an app another run has just frozen. A second manual backup gets `409` with `another backup of this volume is running`, and a scheduled one that overlaps is recorded as failed with the same reason. Each change writes an audit row that says which hook was set or cleared, never the command itself.

`policy set` merges: a flag you leave out keeps the stored value, so setting only `--pre-hook` does not touch the post-hook or the retention counts. Remove a hook on purpose with `--clear-pre-hook` or `--clear-post-hook` (or pass an empty string, as in `--post-hook ''`), and stop pausing with `--no-pause`. If the pre-hook turns saving off, as `save-off` does, always set the matching post-hook in the same change, and check the result with `policy get`.

### Example: a Minecraft world

The backup package knows nothing about game servers. Set these commands on the world volume (replace the RCON password with your own; keep it out of shared shell history):

```bash
levelrail-cli backups volumes policy set mc data \
  --pre-hook 'rcon-cli save-off && rcon-cli save-all flush && sleep 2' \
  --post-hook 'rcon-cli save-on'
```

The pre-command stops autosaves and flushes the world to disk, the archive streams, and the post-command turns saving back on. If RCON is down the backup still runs and carries the `inconsistent:` warning.

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
| `APP_BACKUP_HOOK_TIMEOUT_SECONDS` | `60` | Seeds `backup.hook_timeout_seconds`, the per-hook time limit, clamped by `APP_BACKUP_HOOK_MAX_SECONDS` (`600`). |
| `APP_BACKUP_HOOK_MAX_OUTPUT_BYTES` | `4096` | Seeds `backup.hook_max_output_bytes`, the failing hook output kept after masking. |
| `APP_BACKUP_HOOK_KILL_GRACE_SECONDS` | `5` | Extra wait after a hook's limit for the in-container kill to end it, at least 2. |
| `APP_BACKUP_UPLOAD_ATTEMPTS` | `1` | How many times a failed upload is retried from a fresh archive. |
| `APP_BACKUP_DRILL_INTERVAL_HOURS` | `168` | Scheduled drill period per resource. `0` disables. |
| `APP_BACKUP_DRILL_MEMORY_MB` | `1024` | Memory cap for a scratch database container. It also runs with no network. |
| `APP_BACKUP_DRILL_CPU_MILLI` | `1000` | CPU cap for a scratch container, in thousandths of a core. |

Scratch containers are labelled, and any left by a crash are removed when the control plane starts.
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

### Design note: encrypting database dumps before upload

Volume backups are encrypted on the control plane. Database dumps are not yet, so the bucket sees plaintext SQL and the bucket provider is trusted with it. Planned shape, not built:

- Reuse the age recipient and identity that volume backups already use, so there is one key to escrow.
- Stream the dump through zstd and age before upload, and record the codec on the backup row so older plaintext dumps stay restorable.
- Drills and restores already decode by recorded codec, so the drill path needs no change beyond the decoder.
- Search and replay checks (trailer, table count) run after decryption, never on ciphertext.
- Needs its own security review: key rotation, and what happens to dumps encrypted with a retired key.

## What was tested

These ran against real Docker volumes and a real S3 compatible server (SeaweedFS), no fakes:

| Check | Result |
| --- | --- |
| Back up a volume with a uid 1234:5678 directory (750), a 640 file with a 2020 mtime and a symlink, delete the volume, restore into a new volume | Content hashes, owners, modes, mtime and symlink target identical; manifest digest identical |
| Drill a healthy backup | Passes, scratch volume removed |
| Flip one byte of the stored object | Drill fails at restore, scratch volume removed |
| Truncate the stored object | Drill fails at the object check |
| Restore a truncated object over a live volume | Refused, the live volume is untouched |
| Pre-backup hook, post-backup hook and pause | Hooks ran inside the container, container not left paused, a failing pre-hook aborts the backup, and the post-hook always runs |
| Postgres dump drilled into a scratch Postgres | Restores, two tables counted; a modified dump fails |

Limits: drills for engines other than Postgres, MySQL and MariaDB check the object, not a replay. Object lock and versioning detection depends on the provider answering those API calls; a provider that does not shows as Open.

## Next steps

<CardGroup cols={2}>
<Card title="Backups and storage" href="/backups-and-storage">

Configure backup targets and retention policies.

</Card>
<Card title="Managing databases" href="/managing-databases">

Database backups, restore-as-new, and point-in-time recovery.

</Card>
</CardGroup>
