---
description: Back up and restore the control plane's own database: automatic snapshots, manual snapshots, pre-upgrade safety copies, and the offline restore command.
---

# Control plane backup and restore

The control plane keeps everything it knows (apps, domains, tokens, users, encrypted secrets, deploy history) in one SQLite database. A control plane backup is a consistent snapshot of that file, taken while the server keeps running.

This is separate from [database and volume backups](/backups-and-storage), which protect the data of the apps you deploy.

## What is in a backup, and what is not

| In a backup | Not in a backup |
| --- | --- |
| Every app, domain, environment, project and user record | **The master key** |
| API token hashes and session data | App volumes and managed database contents |
| Secrets, still encrypted | The telemetry (metrics and logs) database |
| Deploy history and audit log | TLS private keys held outside the database |

::: warning The master key is never in a backup
Secrets in the database are encrypted with the master key. A restore without the same master key leaves those secrets unreadable. Keep a separate copy of the master key (a password manager or offline vault), and store it apart from the backups. Whoever holds both can read your secrets.
:::

Backups do contain token hashes and encrypted secrets, so every backup route requires a root-scoped token and files are written with mode `0600`.

## Where backups live

Snapshots are written to `<data dir>/control-plane-backups/` and named `levelrail-YYYYMMDDTHHMMSSZ.db` (UTC). Each one is copied with `VACUUM INTO`, checked with SQLite's `integrity_check`, and hashed with SHA-256.

Snapshots sit on the same disk as the database. To survive losing the machine, download them and store them elsewhere.

## Automatic snapshots

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CONTROL_PLANE_BACKUP_INTERVAL` | `24h` | How often a scheduled snapshot is taken. Go duration syntax. `0` disables scheduled snapshots. |
| `APP_CONTROL_PLANE_BACKUP_RETAIN` | `7` | How many scheduled snapshots to keep. Older scheduled ones are deleted. |

Manual snapshots (created from the CLI or API) and pre-upgrade snapshots are never deleted automatically.

### Doctor check

`levelrail-cli doctor` and the dashboard Status page include a `control_plane_backup` check. It warns when the newest snapshot is older than 3 days (the fix is `levelrail-cli control-plane-backups create`), is ok when a recent one exists, and reports unknown when scheduled snapshots are disabled with `APP_CONTROL_PLANE_BACKUP_INTERVAL=0`. It never fails. See [troubleshooting](/troubleshooting#control-plane-backup-is-stale).

### Before an upgrade

When the server starts on an existing database and this release carries schema migrations that have not been applied yet, it takes a snapshot first. A brand new database is skipped. If that snapshot fails (for example the disk is full), the failure is logged and the migration still proceeds.

### Downgrade guard

If the database has a schema version newer than the binary understands, the server refuses to start and says so. Run a newer release, or restore a snapshot taken by this version.

## Taking and managing backups

```
levelrail-cli control-plane-backups create
levelrail-cli control-plane-backups list
levelrail-cli control-plane-backups download <name> --out backup.db
levelrail-cli control-plane-backups delete <name>
```

Without `--out`, `download` writes the raw bytes to stdout. The same operations are available in the dashboard under Settings, and over the API (see the [API reference](/api-reference)):

| Method | Path |
| --- | --- |
| `POST` | `/api/v1/system/backups` |
| `GET` | `/api/v1/system/backups` |
| `GET` | `/api/v1/system/backups/{name}/download` |
| `DELETE` | `/api/v1/system/backups/{name}` |

## Restoring

Restoring is an offline operation on the server, because the database cannot be swapped under a running control plane.

1. Stop the control plane.
2. Make sure the same master key is available to the restored server.
3. Run the restore against the backup file:

   ```
   levelrail restore-db /path/to/levelrail-20260101T000000Z.db
   ```

   The command uses `APP_DATA_DIR` to find the live database.
4. Start the control plane.

`restore-db` first checks the file: `integrity_check` must pass and its schema version must not be newer than the binary. It then moves the current database aside as `levelrail.db.before-restore-<timestamp>` and puts the backup in place. Nothing is deleted, so a mistaken restore can be undone by moving that copy back.

Anything that changed after the snapshot was taken (new apps, tokens, deploys) is gone after a restore. Running containers are not touched, and the reconciler converges them toward the restored desired state on startup.
