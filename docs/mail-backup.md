# Mail backup and restore

Back up, verify and restore the mox mail server from the dashboard (Mail page, Backup and Storage panels), the CLI (`levelrail-cli mail backup`, `levelrail-cli mail storage`) or the API (`/api/v1/mail/backups`, `/api/v1/mail/storage`). Everything here needs `APP_EXPERIMENTAL=mail`.

## Backup

1. `mox version` is recorded in the manifest (mox prints `v0.0.17-go1.27.0`; the first x.y.z is used).
2. `mox backup /mox/data/tmp/backup/<id>` runs inside the mail container. mox creates the directory itself and writes `config/` and `data/` under it. Staging sits in the data volume so message files are hardlinked, not copied.
3. `mox verifydata /mox/data/tmp/backup/<id>/data` must pass, otherwise nothing is stored.
4. The staging directory is copied out over the Docker API, streamed as tar, then zstd, then age, and stored with a manifest (mox version, time, file count, bytes, sha256 of the archive).
5. Staging is removed on every path, inside the container and on the control plane.

The mail container is found only from platform identity: its exact name from desired state and this instance's reserved label. A run answers at once with a job; poll `GET /api/v1/mail/backups` (the CLI does this with `--wait`). One backup or restore runs at a time per mail app. Jobs are kept in memory and do not survive a control plane restart.

## Encryption

Mail backups use the instance backup encryption key, the same age key that seals volume backups (`APP_BACKUP_ENCRYPTION_KEY_FILE`, default `<data dir>/backup-encryption.key`). Keep a copy of that key off the machine. With `APP_BACKUP_ENCRYPTION=off` mail backups refuse to run, because a mail backup holds every message and password hash.

## Destination

The default destination is a local folder under the control plane data directory, `<data dir>/mail-backups/<app>`. Its cap is `APP_MAIL_BACKUP_LOCAL_CAP_MB` (2048 when unset). After each backup the oldest backups are pruned until the folder fits. The newest verified backup is never pruned. If one backup alone exceeds the cap, the run fails with a cap error and existing backups stay untouched.

You can also save your own S3 or R2 bucket (`mail storage set --kind s3`). Saving runs put, get, list and delete of a small object and shows each step. The secret access key is sealed in the secrets store and never returned. In this release backups are still written to the local folder; the bucket is checked and saved so uploads can use it later.

## Restore

`mail backup restore <id>` without `--confirm` is a dry run: it checks the mox version, the archive checksum and decrypts the whole archive, then reports file and byte counts without writing anything. `mail backup verify <id>` is the same check.

A real restore needs `--confirm <id>`, the root ability and a fresh password step-up. It:

1. Refuses when the running mox is older than the backup's mox.
2. Extracts the backup on the control plane and copies its `data/` into `/mox/data/tmp/restore/<id>` while mail still runs.
3. Stops the mail container, then a short-lived helper container (the mail image, data volume only) moves the live data to `/mox/data/tmp/pre-restore-<id>` and the restored data into place.
4. Starts the mail container again and waits until the admin API answers (`APP_MAIL_RESTORE_WAIT_MINUTES`, default 3).

Only the data directory is restored. The config volume (`mox.conf`, `domains.conf`) stays as it is, so accounts must still exist in the live config. The previous data stays in `tmp/pre-restore-<id>` for a manual rollback until you delete it.

## Limits

- The mail app must run on a node the control plane reaches over Docker exec and archive copy.
- Staging is copied to the control plane disk during a backup, so free space there must cover one backup.
- Run time is bounded by `APP_MAIL_BACKUP_TIMEOUT_MINUTES` and `APP_MAIL_RESTORE_TIMEOUT_MINUTES` (60 each).
- There is no schedule yet; run backups from the dashboard, the CLI or your own cron.
