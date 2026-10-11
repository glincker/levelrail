---
description: "Runbook for upgrading the control plane: checks before, what the self-update does, automatic rollback, interrupted upgrades and the downgrade guard."
---

# Upgrade safely

This is the runbook for moving a running server to a newer release. The self-update verifies the release, snapshots your data, tries the migrations on a copy, and puts the old binary and data back by itself when the new release does not become healthy.

## Before you start

1. Open Settings > Updates, or run `levelrail-cli upgrade --plan`. Read the **breaking changes** between your version and the target. Ones that need acknowledgement block the upgrade until you tick them (dashboard) or pass their ids with `--ack-breaking`.
2. Check the preflight card or `levelrail-cli upgrade`: release signature published, Docker Engine not on the known-bad list, free disk (`APP_UPGRADE_MIN_FREE_BYTES`, 2 GiB default), a recent control plane backup.
3. Make sure the master key is backed up somewhere safe. A snapshot never contains it.

## Run it

Dashboard: Settings > Updates > **Upgrade this server** > Upgrade to vX.Y.Z.

CLI:

```bash
levelrail-cli upgrade --apply [--version vX.Y.Z] [--ack-breaking ID,ID]
```

On the host, as root (the dashboard and CLI call this for you through a transient systemd unit, so stopping the control plane does not stop the upgrade):

```bash
sudo levelrail self-upgrade --to vX.Y.Z [--ack ID,ID] [--timeout 90s]
sudo levelrail self-upgrade --to vX.Y.Z --dry-run     # plan and breaking changes only
```

If the control plane does not run as root or systemd is not the init system, the dashboard and CLI show the exact host command instead of running it.

## What happens

| Step | What it does | If it fails |
| --- | --- | --- |
| guard | Rejects an older or identical target and any unacknowledged breaking change | Refused, nothing changed |
| download | Fetches the binary, `checksums.txt` and the signature bundle over HTTPS (size capped) | Nothing changed |
| verify checksum | SHA-256 of the binary against `checksums.txt` | Refused, nothing changed |
| verify signature | cosign keyless check of `checksums.txt` (`--verify auto` skips when cosign or the bundle is missing, `require` fails closed, `off` skips) | Refused, nothing changed |
| probe | Runs the new binary's `version --json` in an isolated scratch directory and checks the version and that its schema is not older than your database | Refused, nothing changed |
| backup | Snapshots the database (`VACUUM INTO`, integrity checked) and keeps a copy of the running binary | Nothing changed |
| migration dry run | Applies the new release's migrations to a copy of the snapshot with the new binary | Nothing changed, the old version keeps running |
| stop, swap, start | Stops the service, replaces the binary atomically, starts it | Automatic rollback |
| health | Waits for `/healthz` and `/readyz` within the timeout | Automatic rollback |

Nothing on the host changes before the stop step. Everything before it is safe to interrupt.

## Automatic rollback

If a step after the stop fails, or the new release is not healthy within `--timeout` (90 seconds by default):

- the previous binary is restored byte for byte and started;
- if the new release already migrated the database, the pre-upgrade snapshot is put back and the database written by the failed run is kept beside it (`levelrail.db.before-rollback-<timestamp>`);
- if the schema did not change, the database is left alone, so nothing written meanwhile is lost.

The attempt is recorded as `rolled_back` with its step timeline, visible at Settings > Updates and with `levelrail-cli upgrade --attempts`.

What is not kept: data the control plane wrote between the swap and the rollback, only when the schema changed. That window is the health timeout at most. App containers keep running throughout.

If the rollback itself fails, the attempt is `failed` and the message names the previous binary (`<binary>.prev`) and the snapshot, so a person can finish the job by hand with `restore-snapshot`.

## The upgrade was interrupted

An SSH drop or a power cut can end the host command mid-way. The journal in `<data dir>/self-upgrade/` records every step. The next upgrade refuses to start while an attempt is unfinished and tells you to run:

```bash
sudo levelrail self-upgrade --recover
```

Recovery closes an attempt that never stopped the service as failed (nothing to undo), rolls back one that stopped or swapped the binary, and confirms one whose health check had already passed.

## A binary older than the database

Migrations only go forward. A binary whose newest migration is older than the database refuses to start, exits with status 78 and prints:

- its own version and schema, and the database schema and the release that last ran on it;
- the exact command to install that release or newer;
- how to list restore points if you really must run the older binary.

Nothing is modified. The installer's unit sets `RestartPreventExitStatus=78` so systemd does not restart-loop it.

## Release notes metadata

Breaking changes are read from each release's notes. A release author can declare them explicitly with a comment anywhere in the body:

```
<!-- levelrail-upgrade: {"breaking":[{"id":"ingress-ports","summary":"Ingress ports move to 8088/8443 when 80/443 are taken","ack":true}]} -->
```

`ack` defaults to true. Without the comment, every bullet under a heading named "Breaking changes" (including release-please's "BREAKING CHANGES" section) counts and requires acknowledgement. When the notes cannot be fetched the dashboard refuses to start the upgrade and the host command asks for `--skip-notes-check`.

## Mirrors and air-gapped hosts

`LEVELRAIL_RELEASE_BASE_URL` points the download at an https mirror laid out like GitHub release downloads (`<base>/<tag>/<asset>`). `LEVELRAIL_INSECURE_MIRROR=1` allows plain http for a local test mirror. Use `--skip-notes-check` when GitHub's API is unreachable.

## See also

- [Installing](installing.md) for first install, `install.sh --check` and the installer's own `upgrade`
- [Upgrade history](upgrade-history.md) for the record of every version that ran
- [Disaster recovery](disaster-recovery.md) for restoring from backups
