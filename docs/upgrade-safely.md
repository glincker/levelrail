---
description: "Runbook for upgrading the control plane: checks and their fixes, upgrade now, scheduled and automatic upgrades, automatic rollback, interrupted upgrades and the downgrade guard."
---

# Upgrade safely

This is the runbook for moving a running server to a newer release. The self-update verifies the release, snapshots your data, tries the migrations on a copy, and puts the old binary and data back by itself when the new release does not become healthy.

## Before you start

1. Open Settings > Updates, or run `levelrail-cli upgrade --plan`. Read the **breaking changes** between your version and the target. Ones that need acknowledgement block the upgrade until you tick them (dashboard) or pass their ids with `--ack-breaking`.
2. Check the preflight card or `levelrail-cli upgrade`: release signature published, Docker Engine not on the known-bad list, free disk (`APP_UPGRADE_MIN_FREE_BYTES`, 2 GiB default), a recent control plane backup.
3. Make sure the master key is backed up somewhere safe. A snapshot never contains it.

## Fix a blocked check

Every check that blocks an upgrade says what it protects and carries its own fix. The Version card on Settings > Updates lists only the blockers, so you can clear them without leaving it.

| Check | What it protects | Fix |
| --- | --- | --- |
| Recent control plane backup | A restore point for a failed migration. Blocks when the newest backup is older than `APP_UPGRADE_MAX_BACKUP_AGE` (26h default, shown in the message) or none exists. | **Back up now** takes a control plane backup, waits, and re-evaluates the check. CLI: `levelrail-cli upgrade backup-now`. |
| Release checksums and signature | That the binary is the one the project published. | A release younger than `APP_UPGRADE_RELEASE_SETTLE` (1h default) that is missing `checksums.txt` or its signature is **still being published**: it shows as Waiting, not as a failure, and **Re-check** clears it once the assets land. An older release missing them is a hard block until a fixed release ships. |
| Docker Engine version | Not restarting your apps on an engine release with a known bug. | Update Docker on the host, then **Re-check**. |
| Free disk space | Room for the new binary and a snapshot. | Free space, then **Re-check**. |
| Signature verification tool | Verifying the signature as well as the checksum. | Optional: install `cosign` with the command shown. |

Database backups of your own databases are not part of this gate: the upgrade replaces the control plane program and its database, not your data volumes.

## Run it

Dashboard: Settings > Updates > the Version card. When the checks pass, **Upgrade to vX.Y.Z now** opens a summary of the breaking changes to acknowledge, the backup that will be taken and the rollback behaviour. Confirming starts the upgrade, shows its steps live, and reloads the page once the new release answers.

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

## Schedule an upgrade

Pick a time when nobody is deploying. Settings > Updates > **Schedule upgrade**, or:

```bash
levelrail-cli upgrade schedule --at "2026-10-12 03:00" --timezone Europe/Berlin
levelrail-cli upgrade schedule --window            # next maintenance window
levelrail-cli upgrade schedule show
levelrail-cli upgrade schedule cancel
```

The time is read in the server's time zone unless you name one, and the dashboard shows that zone next to the picker. There is at most one schedule at a time; `--replace` (the dashboard's Reschedule) swaps a pending one. `--window` uses the automatic upgrade window if you set one, otherwise the next enabled alert maintenance window that covers everything.

What the scheduler does at the scheduled time, from persisted state, so a control plane restart before then loses nothing:

1. Checks again that the target is still a release on the configured channel and that no new breaking change appeared since you scheduled it.
2. Re-runs the preflight. If only the backup blocks, it takes one and re-checks.
3. If any check still fails it **does not apply**. It records why, raises an attention item and an alert on your notification channels, and retries every `APP_UPGRADE_RETRY_INTERVAL` (2m) until the grace period (`APP_UPGRADE_SCHEDULE_GRACE`, 2h; for a window, the window's end). After that the schedule is marked missed and you are alerted once.
4. If they pass it takes a fresh backup, marks the schedule running, and starts the same engine as Upgrade now. The attempt is recorded with initiator `schedule:<id>`; the schedule becomes applied, or failed when the engine rolled back.

The schedule is marked running before the engine starts, so a crash in between is reported as "did not start" and is never applied twice. A cancel racing a fire resolves to exactly one winner.

Refused at scheduling time: a time in the past or more than `APP_UPGRADE_SCHEDULE_MAX_AHEAD` (90d) away, a target that is not newer or not on the channel, unreadable release notes, unacknowledged breaking changes, and a major version jump without `--ack-major`.

## Automatic upgrades

Off by default. Settings > Updates > **Automatic upgrades**, or:

```bash
levelrail-cli upgrade auto set --mode patch --window-cron "0 3 * * *" --window-duration 2h --timezone UTC --min-age-days 3
levelrail-cli upgrade auto get
levelrail-cli upgrade auto set --pause        # keep the policy, apply nothing
```

| Mode | Applies |
| --- | --- |
| `off` | Nothing. The control plane upgrades only when you start or schedule an upgrade. |
| `patch` | A newer release of the running minor version (v0.5.0 to v0.5.2). |
| `any` | Any newer release on the configured channel, except a major version jump. |

Only releases that need nobody to read them are applied. A release is held, and the reason is shown on the page and sent to your channels, when it has a breaking change that needs acknowledgement, when its notes cannot be fetched, when it is a new major version, or when it is younger than the minimum release age. A release whose automatic attempt failed is not retried automatically. Each qualifying release becomes a normal schedule in the next window, so it goes through the same fire-time checks, backup and rollback as everything above, and pausing or switching off cancels a pending automatic schedule.

## Threat model

Scheduling and the policy are root-only (a root session or a root-ability token), and every change is written to the audit log, as are the scheduler's own actions. A leaked root token could already start an upgrade immediately with `POST /api/v1/updates/self-upgrade`, so scheduling adds no new capability: it only delays when that action happens, and it can be cancelled until it starts. What the policy does add is that a root token can opt a server into applying future releases on its own. The targets are constrained to releases published by the configured repository and verified by the engine (checksum, and signature when `cosign` is installed), breaking changes and major jumps are never applied unattended, and `APP_UPGRADE_*` environment variables, which only the host operator controls, set the bounds. Keep the policy off, or paused, on a server where you want every upgrade to be a decision.

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

## What survives an upgrade

| Item | After the upgrade |
| --- | --- |
| Apps, databases, domains, secrets, settings | Kept, migrated in place |
| Passwords, passkeys, second factors | Kept, sign in as before |
| API tokens (CLI, CI, MCP configs, automations) | Kept: the same token string keeps working with the same abilities, expiry, name and last-used time |
| Revoked or expired API tokens | Stay unusable, never revived |
| Browser sessions | Ended: sign in again |

**API tokens from before the built-in auth library** (releases up to 0.2.0-beta.18) are imported on the first start of the new release, keeping their stored hash, so nothing has to be re-issued. Any token the start-up import missed is imported the first time it is used. Abilities are copied one to one and never widened; a token whose owner no longer holds an ability loses it, as for any token. Tokens that are revoked, expired, belong to a deleted user or carry an unknown ability are skipped. Each import writes an audit log entry (`api_token.legacy_migrated`) that names the token id, never the secret.

The journal shows `legacy tokens migrated migrated=N skipped=M` after the first start, and `levelrail-cli doctor` (or Settings > System status) has a **Pre-upgrade API tokens** check with the same counts. It warns if any owned token is still waiting to be imported.

**Browser sessions** before 0.2.0-beta.19 lived only in the control plane's memory, so there is nothing to carry over (a restart of those releases signed everyone out too). After the upgrade, an open dashboard goes to the sign-in page with a note that the session ended. Sign in again; your password, passkeys and second factors still work.

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
