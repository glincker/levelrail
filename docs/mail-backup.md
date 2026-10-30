# Mail backup core

`internal/mailbackup` is the engine-side core of mail backup and restore. It has no API, schedule or UI yet.

## Backup

1. `mox version` is recorded in the manifest.
2. `mox backup <staging>` runs inside the mail container into an empty staging directory under `mail_data`.
3. `mox verifydata <staging>/data` must pass, otherwise nothing is uploaded.
4. The staging directory is streamed as tar, then zstd, then age to a sink, never held in memory.
5. A manifest (mox version, time, file count, bytes, sha256 of the archive) is stored next to the archive.
6. Staging is removed on every path, success or failure.

## Local folder sink and cap

The local sink keeps backups in one folder. The cap is `APP_MAIL_BACKUP_LOCAL_CAP_MB` (documented default 2048 when unset). After each backup the oldest backups are pruned until the folder fits. The newest verified backup is never pruned. If one backup alone exceeds the cap, the run fails with a typed cap error and existing backups stay untouched.

## Restore

Restore refuses a target mox that is older than the manifest's mox version (no downgrade), verifies the archive sha256 before extracting anything, and extracts into a work directory that is renamed into place only on success. A dry run runs the same checks and reports the file and byte counts without writing.

## Not proven on a real mox

- That `mox backup` accepts a directory created by it inside the container, and that its output layout has `data/` as assumed.
- The exact `mox version` output format (parsed as the first x.y.z).
- Object storage sinks are a later slice.
