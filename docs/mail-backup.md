# Mail backup and restore

Back up, verify and restore the mox mail server from the dashboard (Mail page, Backup and Storage panels), the CLI (`levelrail-cli mail backup`, `levelrail-cli mail storage`) or the API (`/api/v1/mail/backups`, `/api/v1/mail/storage`). Everything here needs `APP_EXPERIMENTAL=mail`.

## Backup

1. `mox version` is recorded in the manifest (mox prints `v0.0.17-go1.27.0`; the first x.y.z is used).
2. `mox backup /mox/data/tmp/backup/<id>` runs inside the mail container. mox creates the directory itself and writes `config/` and `data/` under it. Staging sits in the data volume so message files are hardlinked, not copied.
3. `mox verifydata /mox/data/tmp/backup/<id>/data` must pass, otherwise nothing is stored.
4. The staging directory is copied out over the Docker API, streamed as tar, then zstd, then age, and stored with a manifest (mox version, time, file count, bytes, sha256 of the archive).
5. Staging is removed on every path, inside the container and on the control plane.

Running a backup needs the root ability, because each run can prune older backups. A token with only write or write-sensitive gets 403 `token lacks the required ability (needs root)`.

The mail container is found only from platform identity: its exact name from desired state and this instance's reserved label. A run answers at once with a job; poll `GET /api/v1/mail/backups` (the CLI does this with `--wait`). One backup or restore runs at a time per mail app. Jobs are kept in memory and do not survive a control plane restart.

## Encryption

Mail backups use the instance backup encryption key, the same age key that seals volume backups (`APP_BACKUP_ENCRYPTION_KEY_FILE`, default `<data dir>/backup-encryption.key`). Keep a copy of that key off the machine. With `APP_BACKUP_ENCRYPTION=off` mail backups refuse to run, because a mail backup holds every message and password hash.

## Destination

The default destination is a local folder under the control plane data directory, `<data dir>/mail-backups/<app>`. Its cap is `APP_MAIL_BACKUP_LOCAL_CAP_MB` (2048 when unset). After each backup the oldest backups are pruned until the folder fits. The newest `APP_MAIL_BACKUP_MIN_KEEP` (default 3) verified backups are never pruned, so repeated runs cannot push out every older restore point. If one backup alone exceeds the cap, the run fails with a cap error and existing backups stay untouched.

You can also save your own S3 or R2 bucket (`mail storage set --kind s3`). Saving runs put, get, list and delete of a small object with the new key first and shows each step; a failed check keeps the previous working settings. The secret access key is sealed in the secrets store and never returned. In this release backups are still written to the local folder; the bucket is checked and saved so uploads can use it later.

## Restore

`mail backup restore <id>` without `--confirm` is a dry run: it checks the mox version, the archive checksum and decrypts the whole archive, then reports file and byte counts without writing anything. `mail backup verify <id>` is the same check.

A real restore needs `--confirm <id>`, the root ability and a fresh password step-up. It decrypts the archive once, checking its checksum in the same pass, and:

1. Refuses when the running mox is older than the backup's mox, or when either version is not a mox release version.
2. Extracts the backup on the control plane and copies its `data/` (and `config/`, for backups that carry it) into `.levelrail/restore-<id>` on the same volume while mail still runs, then removes the control plane copy.
3. Runs `mox verifydata` on the staged data. A failure stops here and nothing live changes.
4. Sets a maintenance hold on the mail app before it stops the container. The hold is stored with the app, survives a control plane restart and shows in `mail status`; while it is set the platform does not start, recreate or remove the app's containers.
5. A helper container (the mail image, no network, only the mail volumes) moves the live entries to `.levelrail/pre-restore-<time>-<id>` and the staged entries into place, writing a journal (`.levelrail/swap`) before each step. The container must still be stopped before and after every step.
6. Starts mail and waits until the admin API answers (`APP_MAIL_RESTORE_WAIT_MINUTES`, default 3), then closes the journal and releases the hold.

Any failure rolls the swap back from the pre-restore copy and starts mail on the previous data; the job reports `rollback: rolled_back`, or `rollback_failed`, in which case the app stays held and the previous data stays in `.levelrail/` on its volumes. If the control plane stops mid-restore, it finds the hold when it starts again and runs the recovery once in the background (logged, bounded by `APP_MAIL_RESTORE_TIMEOUT_MINUTES`): it finishes the restore when the data journal is closed and rolls it back otherwise. With no hold this is one read of the app list. If the mail node is not reachable yet at start up, the next mail backup call runs the same recovery.

The swap script runs under the mail image's BusyBox `sh` (mox v0.0.17 is Alpine with BusyBox 1.37). `TestLiveRestoreSwap` (set `MOXRUN_LIVE_SWAP_IMAGE` to the pinned image) proves it there on real volumes with file names holding spaces, dotfiles, a leading dash, nested dirs, symlinks and an existing `.levelrail` dir: the success path, a second restore with the same id, a failure and a kill at every external command of the swap and of the rollback, and a `docker kill` of the helper mid-swap. Each failure case ends with the original tree restored, compared by a hash of every path, mode, file sha256 and link target. A rollback interrupted part way is safe to run again. The live `tmp` dir stays in place and a staged `tmp` is never moved in.

`mox serve` does not clear `data/tmp` on start up (checked on v0.0.17: files in `data/tmp` survive a start; mox only fixes their permissions), so leftover restore staging there is removed by the sweep below, not by mox.

Backups include `/mox/config` (DKIM private keys, certificates, `mox.conf`, `domains.conf`) next to `/mox/data`, and a restore swaps both under the same hold and rollback. Backups made before this change carry only the data directory, and restoring one leaves the config volume as it is. The archive is encrypted with the instance backup key, so keep that key off the machine.

`APP_MAIL_RESTORE_KEEP_PRE_RESTORE` (default 2) caps how many pre-restore copies stay on the volume; the oldest goes only after a later restore succeeds. `mox backup` runs inside `mox serve` as the mox user (uid 1000) and walks both volume roots, so `.levelrail/` exists only from the start of a swap, is owned like the volume root with mode 0700, and is removed again once it is empty. Recovery with nothing to recover, backups, list, verify and dry runs create nothing on the volumes. Before each backup or restore the sweep refuses while a swap journal is open ("a restore is in progress or needs recovery"), removes an empty work dir and hands root-owned dirs left by older releases to the mox user. Kept pre-restore copies are included in later backups (mox reports them as unrecognized files), so set the keep count to 0 to keep backups small. A restore does not put the backup's `moxversion` file in the live data dir, because `mox backup` fails when one is there. Leftover staging and discarded copies on the volume and plaintext staging on the control plane older than `APP_MAIL_RESTORE_SWEEP_MINUTES` (default 120) are removed at start up and before each backup or restore.

## Limits

- The mail app must run on a node the control plane reaches over Docker exec and archive copy.
- Staging is copied to the control plane disk during a backup, so free space there must cover one backup.
- Run time is bounded by `APP_MAIL_BACKUP_TIMEOUT_MINUTES` and `APP_MAIL_RESTORE_TIMEOUT_MINUTES` (60 each).
- There is no schedule yet; run backups from the dashboard, the CLI or your own cron.
