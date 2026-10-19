---
title: "Rolling back the control plane"
description: "How to roll the control plane back to a previous release: what the dashboard shows and what you run on the server."
"og:url": "https://docs.levelrail.com/rollback"
---

Settings > Updates lists the last five releases for a channel (stable, beta, or
both), each with a compatibility verdict against your database. Pick one to see
exactly what a rollback would do. When the verdict is "Safe: binary only", the
release is kept on this host, the preflight passes and the control plane runs
as root under systemd, the plan shows a **Roll back now** button (root only,
type the version to confirm). The control plane hands the swap to a transient
host unit, because it cannot replace the binary it runs from; it takes a backup,
swaps, restarts and restores the previous release if the old one does not
become healthy. Anything that would restore a database backup stays a typed
host command, since it loses data.

```sh
levelrail-cli upgrade --rollback v0.2.0-beta.14 --yes
```

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

Releases your host kept (the last three it ran) roll back immediately, with no network needed:

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
it and waits for health and readiness. If the target is not healthy within
`--timeout` (default 90s), the previous release is put back automatically and
the command tells you so. Every attempt is written to the audit log.

### When a restore is required

```sh
sudo levelrail rollback --to v0.1.0 \
  --restore-backup levelrail-20261009T020000Z.db \
  --confirm-data-loss levelrail-20261009T020000Z.db
```

The plan lists each local backup with its time and schema. Everything written
after the chosen backup is lost. Your current database is kept beside it as
`levelrail.db.before-rollback-<time>`. Without a service manager, pass
`--stop-cmd` and `--start-cmd`.

## After an upgrade or rollback

Settings > Updates > Verify runs the control plane's own health checks and
shows a pass or fail result with the failing checks listed.

## Troubleshooting

### Rollback command fails with "version not available locally"

**Likely cause:** The control plane does not have the target release cached locally. Only the last three releases it ran are kept.

**How to confirm:** Run `levelrail-cli upgrade --list` and check which versions are marked as available locally.

**Fix:** Use the curl command shown in the "Applying on the server" section to download and verify the release first, then run the rollback command again.

### Rollback times out waiting for health

**Likely cause:** The older release does not become healthy within the timeout (default 90 seconds), possibly due to slow disk or database checks.

**How to confirm:** Run the command and watch for a timeout message, or check the service logs with `journalctl -u levelrail -f`.

**Fix:** Rerun with `--timeout 300` for a longer wait, or check the logs for errors in the service startup. If the release is genuinely incompatible, the automatic rollback will put the previous version back in place.

### Database requires a restore but backups are missing

**Likely cause:** No local backups exist, or they are older than the database schema change.

**How to confirm:** Run `sudo levelrail rollback --to <version> --dry-run` to see the plan. It will list available backups or state that none are available.

**Fix:** If backups exist but are from before the schema change, restore from an S3 or other backup target using `levelrail-cli backups restore`. If no backups are available, upgrading again is safer than a forced restore.

### Still stuck

Check the audit log in Settings > Audit Log for details of the last rollback attempt, including any error messages. Or run `journalctl -u levelrail -n 100` on the server to see the service startup logs.
