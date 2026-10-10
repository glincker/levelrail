# Rolling back a release

Settings > Updates lists the last five releases for a channel (stable, beta, or
both) with a compatibility verdict against your database. Picking one shows
exactly what a rollback would do. Nothing is applied from the dashboard: the
control plane never replaces its own binary, so you run the rollback on the
server.

## What the verdict means

| Verdict | Meaning |
| --- | --- |
| Safe: binary only | The release supports your database schema as it is. Only the program is replaced and your data is untouched. |
| Newer schema | The release is newer than your database and will migrate it forward on start. That is an upgrade. |
| Needs a backup restore | The release is older than your database schema and would refuse to start. The only way back restores a backup and permanently loses everything written after it. |
| Compatibility unknown | The release does not publish its schema version. An older program refuses a newer schema without touching data, and the previous release is restored automatically if it fails to start. |

## From the CLI

```sh
levelrail-cli upgrade --list --channel all
levelrail-cli upgrade --rollback-plan v0.2.0-beta.14 --json
```

## Applying on the server

Releases your host kept (the last three it ran) roll back instantly and offline:

```sh
sudo levelrail rollback --to v0.2.0-beta.14 --dry-run   # print the plan only
sudo levelrail rollback --to v0.2.0-beta.14             # asks you to type the version
```

A release the host does not have is staged first, with checksum and signature
verification:

```sh
curl -fsSL https://raw.githubusercontent.com/glincker/levelrail/main/install.sh \
  | sudo LEVELRAIL_VERSION=v0.2.0-beta.14 sh -s retain
```

The rollback takes a fresh backup, stops the service, swaps the binary, starts
it and waits for health and readiness. If the target does not become healthy
within `--timeout` (default 90s) the previous release is put back automatically
and the command reports that. Every attempt is written to the audit log.

### When a restore is required

```sh
sudo levelrail rollback --to v0.1.0 \
  --restore-backup levelrail-20261009T020000Z.db \
  --confirm-data-loss levelrail-20261009T020000Z.db
```

The plan lists each local backup with its time and schema. Everything written
after the chosen backup is lost; the database you have now is kept beside it as
`levelrail.db.before-rollback-<time>`. Without a service manager, pass
`--stop-cmd` and `--start-cmd`.

## After an upgrade or rollback

Settings > Updates > Verify runs the control plane's own health checks and
shows a pass or fail result with the failing checks listed.
