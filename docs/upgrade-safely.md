---
title: "Upgrade safely"
description: "Upgrade the control plane safely with pre-flight checks and automatic rollback."
"og:url": "https://docs.levelrail.com/upgrade-safely"
---

This is the runbook for moving a running server to a newer release. The self-update verifies the release, snapshots your data, tries the migrations on a copy, and puts the old binary and data back by itself when the new release does not become healthy.

## Before you start

1. Open Settings > Updates, or run `levelrail-cli upgrade --plan`. Read the breaking changes between your version and the target. Ones that need acknowledgement block the upgrade until you tick them (dashboard) or pass their ids with `--ack-breaking`.
2. Check the preflight card or `levelrail-cli upgrade`: release signature published, Docker Engine not on the known-bad list, free disk (`APP_UPGRADE_MIN_FREE_BYTES`, 2 GiB default), a recent [control plane backup](/control-plane-backup).
3. Make sure the master key is backed up somewhere safe. A snapshot never contains it.

## Fix a blocked check

Every check that blocks an upgrade says what it protects and carries its own fix. The Version card on Settings > Updates lists only the blockers, so you can clear them without leaving it.

| Check | What it protects | Fix |
| --- | --- | --- |
| Recent control plane backup | A restore point for a failed migration. Blocks when the newest backup is older than `APP_UPGRADE_MAX_BACKUP_AGE` (26h default, shown in the message) or none exists. | **Back up now** takes a control plane backup, waits, and re-evaluates the check. CLI: `levelrail-cli upgrade backup-now`. |
| Release checksums and signature | That the binary is the one the project published. | A release younger than `APP_UPGRADE_RELEASE_SETTLE` (1h default) that is missing `checksums.txt` or its signature is still being published: it shows as Waiting, not as a failure, and **Re-check** clears it once the assets land. An older release missing them is a hard block until a fixed release ships. |
| Compatible with your database | That you are never offered a release older than your database, which would refuse to start. The release publishes its schema version in `release-manifest.json`, covered by the signed `checksums.txt`; the server fetches that small file (64 KiB cap, GitHub only), verifies it, and compares it with your database. | An older release is blocked, is not offered as an upgrade, and says: `target release is older than the database schema: database is at schema N, vX supports M`. Stay on your current version, or use **Roll back** (it scrolls to Release history) to go to an older release on purpose. A release that publishes no manifest shows as unknown and is checked after download instead. CLI: `levelrail-cli upgrade` prints the same check. |
| Docker Engine version | Not restarting your apps on an engine release with a known bug. | Update Docker on the host, then **Re-check**. |
| Free disk space | Room for the new binary and a snapshot. | Free space, then **Re-check**. |
| Release signature verification | The server can check the release signature itself. It needs one download of Sigstore trust data, which starts automatically and is then saved for offline use. | Nothing to install. While it downloads you see a retry note; use **Re-check** if you are waiting. |
| Release signature pin | That a release with its signature stripped is not accepted, once this host has verified a signed one. | Not pinned yet: a note that the next upgrade verifies and pins. Pinned: blocks when the release publishes no signature, or the Sigstore trust data has never been downloaded. See [Signature pin](#signature-pin). |

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

### One upgrade at a time

The server decides whether an upgrade is running, so a reload, another tab or the CLI all see the same answer. While one runs, every page shows a slim bar ("Upgrading to vX.Y.Z, step 3 of 6"), and the Updates page shows the live stages instead of the Upgrade button. If the control plane restarts mid-upgrade the bar says it is reconnecting and carries on once the new release answers. Starting a second upgrade is refused with `409` and the running attempt id.

When the attempt finishes, the Updates page keeps a compact result card: "Upgraded from vA to vB at TIME, verification passed" with a link to the release notes and **View attempt details** for the stage timeline. A failed attempt shows the reason with the Fix, Retry and Roll back actions. Dismissing the card is remembered in this browser. A one-time notification appears on the next page load, and the Inbox gets an "Upgrade to vB succeeded" item that you can archive like other resolved items. No email is sent for this.

```bash
levelrail-cli upgrade status    # prints the running attempt and the last finished one
```

The same data is at `GET /api/v1/updates/self-upgrade/status`.

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
3. If any check still fails it does not apply. It records why, raises an attention item and an alert on your notification channels, and retries every `APP_UPGRADE_RETRY_INTERVAL` (2m) until the grace period (`APP_UPGRADE_SCHEDULE_GRACE`, 2h; for a window, the window's end). After that the schedule is marked missed and you are alerted once.
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

Automatic upgrades apply only releases that need no human to read the notes first. A release is held, and the reason is shown on the page and sent to your channels, when it has a breaking change that needs acknowledgement, when its notes cannot be fetched, when it is a new major version, or when it is younger than the minimum release age. A release whose automatic attempt failed is not retried automatically. Each qualifying release becomes a normal schedule in the next window, so it goes through the same fire-time checks, backup and rollback as everything above, and pausing or switching off cancels a pending automatic schedule.

## Threat model

Scheduling and the policy are root-only (a root session or a root-ability token), and every change is written to the audit log, as are the scheduler's own actions. A leaked root token could already start an upgrade immediately with `POST /api/v1/updates/self-upgrade`, so scheduling adds no new capability: it only delays when that action happens, and it can be cancelled until it starts. What the policy does add is that a root token can opt a server into applying future releases on its own. The targets are constrained to releases published by the configured repository and verified by the engine (checksum, and signature when `cosign` is installed), breaking changes and major jumps are never applied unattended, and `APP_UPGRADE_*` environment variables, which only the host operator controls, set the bounds. Keep the policy off, or paused, on a server where you want every upgrade to be a decision.

## What happens

| Step | What it does | If it fails |
| --- | --- | --- |
| guard | Rejects an older or identical target and any unacknowledged breaking change | Refused, nothing changed |
| download | Fetches the binary, `checksums.txt` and the signature bundle over HTTPS (size capped) | Nothing changed |
| verify checksum | SHA-256 of the binary against `checksums.txt` | Refused, nothing changed |
| verify signature | keyless Sigstore check of `checksums.txt`, done inside the server (`--verify auto` skips when the bundle or the trust data is missing until this host is [pinned](#signature-pin), then fails closed; `require` fails closed, `off` skips) | Refused, nothing changed |
| probe | Runs the new binary's `version --json` in an isolated scratch directory and checks the version and that its schema is not older than your database | Refused, nothing changed |
| backup | Snapshots the database (`VACUUM INTO`, integrity checked) and keeps a copy of the running binary | Nothing changed |
| migration dry run | Applies the new release's migrations to a copy of the snapshot with the new binary | Nothing changed, the old version keeps running |
| stop, swap, start | Stops the service, replaces the binary atomically, starts it | Automatic rollback |
| health | Verifies within the window (`APP_UPGRADE_VERIFY_WINDOW`, 3 minutes by default): `/readyz` passes with the reconcile loop running, the database is at the new release's schema head, and agents that were connected before the upgrade have reconnected | Automatic rollback |

Nothing on the host changes before the stop step. Everything before it is safe to interrupt.

## Automatic rollback

If a step after the stop fails, or verification does not pass within `--timeout` (the verify window, 3 minutes by default):

- the previous binary is restored byte for byte and started;
- if the new release already migrated the database, the pre-upgrade snapshot is put back and the database written by the failed run is kept beside it (`levelrail.db.before-rollback-<timestamp>`);
- if the schema did not change, the database is left alone, so nothing written meanwhile is lost.

The attempt is recorded as `rolled_back` with its step timeline, visible at Settings > Updates and with `levelrail-cli upgrade --attempts`.

What is not kept: data the control plane wrote since the pre-upgrade snapshot, only when the schema changed. The attempt records how many minutes of changes that was (shown on the attempt and in `upgrade --attempts`), so nothing is lost silently. App containers keep running throughout.

If the rollback itself fails, the attempt is `failed` and the message names the previous binary (`<binary>.prev`) and the snapshot, so a person can finish the job by hand with `restore-snapshot`.

## Restore points

Before every upgrade a restore point is kept under `<data dir>/restore-points`: a consistent database copy, the running binary and the service unit file (files are mode 0600). The newest `APP_UPGRADE_RESTORE_POINTS` points (3 by default) are kept within `APP_UPGRADE_RESTORE_POINTS_MAX_MB` (2048 by default); the oldest are pruned first and the newest always stays.

Settings > Updates lists them under the rollback section with a **Restore** button, and the CLI has `levelrail-cli updates restore-points list` and `levelrail-cli updates restore-points restore <id> --yes`. Restoring puts back the binary and unit file, and the database only when it was migrated past the restore point; the result says how many minutes of changes that discards. It is root only, audited, and refuses while an upgrade is running. On the host: `sudo <binary> restore-point --id <id>`.

## Breaking defaults

Three defaults became stricter. Each has an override, so a deployment that relied on the old behavior can keep it until it is ready.

| Default | Was | Is now | Keep the old behavior |
| --- | --- | --- | --- |
| Docker API guard (`APP_DOCKER_GUARD`) | `audit`: record, never block | `enforce`: a request a rule denies gets `403` | `APP_DOCKER_GUARD=audit`, or `off`. A mode saved from the dashboard is kept as is. |
| Agent enrollment without a CSR | accepted: the control plane generated the agent's key | refused for good: the agent must be upgraded. Nodes holding a control-plane key show "re-enroll required" and are not renewed. | None. Upgrade the agent, then Re-enroll the node. |
| Dashboard listener (`APP_HTTP_ADDR`) | `:8080`, all interfaces | `127.0.0.1:8080` only for a bare binary outside a container whose `APP_DASHBOARD_URL` is an `https://` URL; `:8080` everywhere else, with a one-time warning when it is reachable beyond this host | `APP_HTTP_ADDR=:8080` |

`install.sh` and the container image set `APP_HTTP_ADDR` themselves, so neither changes. After upgrading, `levelrail-cli doctor` shows the guard mode and any request it denied.

## What survives an upgrade

| Item | After the upgrade |
| --- | --- |
| Apps, databases, domains, secrets, settings | Kept, migrated in place |
| Passwords, passkeys, second factors | Kept, sign in as before |
| API tokens (CLI, CI, MCP configs, automations) | Kept: the same token string keeps working with the same abilities, expiry, name and last-used time |
| Revoked or expired API tokens | Stay unusable, never revived |
| Browser sessions | Ended: sign in again |

API tokens from before the built-in auth library (releases up to 0.2.0-beta.18) are imported on the first start of the new release, keeping their stored hash, so nothing has to be re-issued. Any token the start-up import missed is imported the first time it is used. Abilities are copied one to one and never widened; a token whose owner no longer holds an ability loses it, as for any token. Tokens that are revoked, expired, belong to a deleted user or carry an unknown ability are skipped. Each import writes an audit log entry (`api_token.legacy_migrated`) that names the token id, never the secret.

The journal shows `legacy tokens migrated migrated=N skipped=M` after the first start, and `levelrail-cli doctor` (or Settings > System status) has a **Pre-upgrade API tokens** check with the same counts. It warns if any owned token is still waiting to be imported.

Browser sessions before 0.2.0-beta.19 lived only in the control plane's memory, so there is nothing to carry over (a restart of those releases signed everyone out too). After the upgrade, an open dashboard goes to the sign-in page with a note that the session ended. Sign in again; your password, passkeys and second factors still work.

## The upgrade was interrupted

An SSH drop or a power cut can end the host command mid-way. The journal in `<data dir>/self-upgrade/` records every step. The next upgrade refuses to start while an attempt is unfinished and tells you to run:

```bash
sudo levelrail self-upgrade --recover
```

Recovery closes an attempt that never stopped the service as failed (nothing to undo), rolls back one that stopped or swapped the binary, and confirms one whose health check had already passed.

## Signature pin

The first verified release signature writes `<data dir>/release-signature.pin`. After that every upgrade, from the dashboard, `levelrail-cli upgrade --apply`, `sudo levelrail self-upgrade` or `install.sh upgrade`, requires a valid signature: a missing bundle (an unsigned or pre-signing release, including a downgrade to one) or Sigstore trust data that cannot be loaded is refused with the reason and the way out. A bad signature is always refused, pinned or not.

`levelrail-cli doctor` (check `signature_pin`) warns while the host is not pinned, and Settings > Updates notes it. Upgrading once verifies the signature and pins the host.

To get past a pinned refusal on purpose, both of these are logged:

```bash
sudo levelrail self-upgrade --to v1.2.3 --verify off   # this run only, the pin stays
sudo levelrail self-upgrade --unpin-signature          # forget the pin
```

## A binary older than the database

Migrations only go forward. A binary whose newest migration is older than the database refuses to start, exits with status 78 and prints:

- its own version and schema, and the database schema and the release that last ran on it;
- the exact command to install that release or newer;
- how to list restore points if you really must run the older binary.

The Updates page and `levelrail-cli upgrade` catch this before you click: see the Compatible with your database check above.

Nothing is modified. The installer's unit sets `RestartPreventExitStatus=78` so systemd does not restart-loop it.

## What is tested on every release

An automated matrix builds a control plane database at several older schema versions, fills it with apps, environment values, secrets, domains, deploys, users, API tokens, backup targets, audit rows, plugins and model endpoints, then upgrades it to the current schema through the same start-up path a real upgrade uses. For each starting version it checks that:

- the upgrade finishes and no table loses rows;
- secrets still decrypt with the same master key, the audit hash chain still verifies and API tokens still authenticate;
- the control plane answers `/healthz` and `/readyz` on the migrated database;
- a second start does nothing, and takes no new snapshot;
- the snapshot taken before the migration holds the old schema, and restoring it brings back the old data, which then upgrades again cleanly.

Two failure cases are covered as well. A binary older than the database refuses to start, exits with status 78 and leaves the database file byte for byte unchanged. A migration that fails partway keeps the steps already committed, leaves the snapshot intact, and sends the control plane into safe mode with a migration failure reason, where the snapshot can be restored.

The two most recent starting versions run on every pull request. The full set runs in the nightly suite. Run `scripts/upgrade-fixtures/generate.sh` to write the seeded pre-upgrade databases to a folder for inspection.

## Upgrading real releases with real data

The matrix above upgrades synthetic databases in process. The release harness goes further: it runs the actual binary of an earlier release, so the old code writes the data and the old `self-upgrade` command performs the upgrade.

```bash
scripts/upgrade-harness.sh v0.8.2 v0.8.1
```

Each tag is built from its own source (an archive of the tag plus `go build`, in a temporary directory) or taken from `LEVELRAIL_UPGRADE_FROM_BINARY_DIR/<tag>/levelrail`. The test is `TestReleaseUpgradeHarness` in `cmd/levelrail`; it is skipped unless `LEVELRAIL_UPGRADE_FROM_TAGS` is set and under `go test -short`. `LEVELRAIL_UPGRADE_TARGET_VERSION` names the version the current build reports (default `v0.8.99`). It needs about 2 GiB of disk and several minutes per tag.

For each tag the harness:

1. starts the old release on an empty data directory and seeds it through its HTTP API: an app, an app secret, a database record, a second user and the audit rows those calls produce, plus a probe row written straight into the database;
2. upgrades to the current build with `self-upgrade` and checks that the app, secret value (decrypted with the same master key), database record, user and probe row are unchanged, that no audit row was lost and that the audit chain verifies;
3. checks the restore point: it holds the previous release's binary byte for byte and a database copy that passes `PRAGMA integrity_check` and contains the seeded app;
4. creates a change after the upgrade, then restores the point with `levelrail restore-point` and checks the old version is running again with the seeded data intact;
5. upgrades to a release that rewrites the database and never becomes healthy, and checks the automatic restore: the previous binary is back byte for byte, the database is the pre-upgrade one and every seeded record is intact;
6. upgrades again, then attempts a downgrade with `self-upgrade --to <old tag>`, which is refused without touching the binary. It then marks the database as written by a newer release and starts the old binary on it: the binary must exit with status 78, leave the database file byte for byte unchanged, and the current release must start again once the marker is removed.

### Cross-release schema migration

`TestReleaseUpgradeSchemaMigration` and `TestReleaseUpgradeSchemaAutoRestore` (same switch, same `scripts/upgrade-harness.sh`) upgrade a data directory that the real v0.8.2 binary created to the current build. v0.8.2 ships migrations up to 2830 and the current tree ships more, so the upgrade runs real migrations on real data. It seeds a legacy single-row email backend (SMTP, with a stored password) and an app spend cap through the v0.8.2 API, sends a test email through a local SMTP sink to prove the credential works, and then checks:

1. email: `prov_primary` exists in `email_providers` with every non-secret column equal to the old `email_settings` row, `GET /api/v1/settings/email` and `GET /api/v1/settings/email/providers` agree, the primary reports its credential as set, and a test send through the primary reaches the sink with the original password. The deprecated `PUT /api/v1/settings/email` still round-trips, keeps the credential when none is sent, and both views follow it;
2. spend caps: the cap row and its history survive unchanged, `guard_mode` is `alert`, no budget or override is blocking, and the app's deploy guard reports allowed;
3. migrations: `schema_migrations` holds exactly the shipped list (no gaps, no extras, names match), the migrations the old release had are not applied again, the new ones carry an `applied_at` from the upgrade, and restarting the current build against the same data directory changes nothing and takes no snapshot;
4. downgrade: the v0.8.2 binary started on the upgraded database exits with status 78, prints the documented message, and leaves the database file (and its WAL) byte for byte unchanged. `self-upgrade --to v0.8.2` is refused too;
5. restore: the pre-upgrade restore point holds the v0.8.2 binary and a database at the old schema with no `email_providers` table. In `TestReleaseUpgradeSchemaAutoRestore` the current build migrates for real, its health check is made to fail, and the v0.8.2 `self-upgrade` rolls back on its own: the old binary, schema, migration list, legacy email row, cap and probe row all come back, v0.8.2 sends the test email again, and a later upgrade migrates the restored data cleanly.

Known gap: restoring that point by hand with the current binary (`restore-point`) swaps the v0.8.2 binary and database back, and then the command opens the live database to record the attempt, which migrates the restored database forward again while v0.8.2 is running. The step is kept in the test, skipped with the reason, and runs when `LEVELRAIL_UPGRADE_RUN_KNOWN_GAPS=1` is set. The automatic restore is not affected because the old binary does the recording.

`TestUpgradeSchemaMigrationInProcess` is the fast version and runs under `go test -short`: it builds the schema v0.8.2 shipped from the real migration files, seeds a legacy email row and a cap, migrates to the current tree and checks the backfill, the `alert` default, the migration list and that a reopen is a no-op. It does not use a release binary.

### What this does and does not prove

Proven for v0.8.1 and v0.8.2 as the starting release: data written by the old binary survives the upgrade, a failed verification puts back the old binary and database, a restore point restores a working old release, and a downgrade is refused by both the upgrade command and the old binary's start-up guard.

Limits:

- The newer-database state in step 6 of the first test is simulated with a marker row. The schema migration tests above use the real schema difference instead.
- A manual restore point restore only swaps the database when the schema differs, so with equal schemas it keeps the database ("database kept") and the harness checks exactly that. The database swap on restore is proven by step 5 (automatic restore), where the failing release rewrites the database.
- No containers run: the harness uses the control plane in dev mode without Docker, so app deploys, agent reconnection and the unit file restore (systemd) are not exercised. The agent reconnect part of the verification is skipped because no agent is enrolled.
- The release is served from a local test server over plain HTTP with `LEVELRAIL_INSECURE_MIRROR`, so the signature check reports "no signature bundle, checksum only". Signature verification has its own tests.
- The harness runs on demand, on a developer machine or a CI job. It is not part of the pull request lane.

## What is checked before a release is published

`release.yml` runs a release gate before any binary, package or image is published. It fails the release when:

- the release manifest does not name the tag and the schema version the candidate binary reports;
- the candidate's schema is lower than the previous published release's, because installs of that release could never upgrade to it;
- the previous release's binary, started on seeded data, cannot be stopped and replaced by the candidate on the same data directory, healthy on `/readyz` and at the candidate's schema version;
- the candidate starts on a database newer than itself instead of exiting with status 78.

The first release after this gate existed has no earlier schema manifest to compare, so a missing previous release is a warning, and a previous release without a manifest is compared through its own `version --json`. Run `scripts/release-gate.sh --help` to rehearse it locally against two binaries, and `scripts/test-release-gate.sh` to test the gate itself.

## Release notes metadata

Breaking changes are read from each release's notes. A release author can declare them explicitly with a comment anywhere in the body:

```
<!-- levelrail-upgrade: {"breaking":[{"id":"ingress-ports","summary":"Ingress ports move to 8088/8443 when 80/443 are taken","ack":true}]} -->
```

`ack` defaults to true. Without the comment, every bullet under a heading named "Breaking changes" (including release-please's "BREAKING CHANGES" section) counts and requires acknowledgement. When the notes cannot be fetched the dashboard refuses to start the upgrade and the host command asks for `--skip-notes-check`.

## Channels and the edge channel

Settings > Updates shows the active channel on the Version card with a one-line description, and the same descriptions sit under the channel picker.

| Channel | What it follows | How it installs |
| --- | --- | --- |
| Stable | The latest non-prerelease GitHub release | Signed binary, checksums and signature verified |
| Beta | The newest pre-release | Signed binary, checksums and signature verified |
| Edge | The newest commit on `main`, shipped as the moving `edge` container image tag | Pull the image and recreate the container |

Edge is not a GitHub release. It has no binaries, no `checksums.txt` and no signature, so the "Release checksums and signature" preflight check is shown as not applicable instead of blocking, and the card says so. The version shows as `main-<short sha>` and links to the commit.

What edge means for your install:

- If the control plane runs as a container, the preflight card shows the exact image (`ghcr.io/<owner>/<name>:edge`) with the `docker pull` command and the compose pull and recreate command. `levelrail-cli upgrade` prints the same.
- If it runs as a host binary under systemd, edge is not installable that way. The card offers one-click buttons to move the channel to Beta or Stable (`levelrail-cli settings updates set --channel beta` does the same).

Self-upgrade (`--apply`, and the dashboard Upgrade button) only ever applies a release tag, so it refuses a `main-<sha>` target. The same holds for the rest of this page on the edge channel:

- Upgrade now and Schedule upgrade are hidden. The Version card shows the pull guidance instead. A schedule request for an edge target is refused with a 409 that says edge builds are container images.
- Automatic upgrades cannot be turned on. The policy is refused while the channel is edge, and a policy that was already on records "held: edge builds are container images" instead of scheduling anything.
- A schedule created earlier is re-checked at fire time. If its target turns out to be an edge image it is marked failed and nothing is applied.
- A stable or beta release that is still being published shows as Waiting with a Re-check button. Only edge reports checks as Not applicable.

## Mirrors and air-gapped hosts

`LEVELRAIL_RELEASE_BASE_URL` points the download at an https mirror laid out like GitHub release downloads (`<base>/<tag>/<asset>`). `LEVELRAIL_INSECURE_MIRROR=1` allows plain http for a local test mirror. Use `--skip-notes-check` when GitHub's API is unreachable.

## Troubleshooting

### Upgrade fails and rolls back automatically

The new release was downloaded and started, but did not become healthy within the timeout window.

Likely cause: a schema migration failed, the new release has a bug, or the timeout is too short for the system's speed.

How to confirm: check Settings > Updates > Upgrade attempts to see the exact step that failed and any error output. Also check `sudo journalctl -u levelrail -n 100` to see startup logs and any database errors.

Fix: if the error is a database issue (migration failure), a backup was taken and can be restored. If it is a code bug, wait for a fixed release and try again. If the timeout is insufficient (slow disk, large database), increase `--timeout` when running the self-upgrade and try again.

### A schema migration blocks the upgrade

The preflight check is blocking because a breaking change requires manual action before upgrading.

Likely cause: the new release contains a database schema change that needs acknowledgement or preparation.

How to confirm: open Settings > Updates and scroll to the Version card. It lists all breaking changes that need acknowledgement by clicking the checkbox next to each one. Or run `levelrail-cli upgrade --plan` to see the list.

Fix: read each breaking change description in the card (or CLI output) to understand what it means, then acknowledge it by checking the checkbox or passing `--ack-breaking <id>` to the CLI. The change tells you any pre-upgrade steps needed (e.g., backing up certain data). Once all are acknowledged, the upgrade proceeds.

### The old binary refuses to start after a failed upgrade

The control plane exited with status 78 and printed that the database is newer than the binary.

Likely cause: a migration ran in the new version but that version crashed, and now the rolled-back old binary cannot read the migrated schema.

How to confirm: check `sudo systemctl status levelrail` and `sudo journalctl -u levelrail -n 20`. The output names the version that last ran successfully and the current version.

Fix: there is no going back. Install the new release again with `curl -fsSL https://levelrail.com/install.sh | sudo LEVELRAIL_VERSION=<new_version> sh`, which will be the only binary that can read the current database, or restore a backup from before the failed upgrade with `levelrail restore-snapshot <timestamp>` (and keep the old binary for the old schema). See [Rolling back](/installing#rolling-back).

### The control plane does not restart after an upgrade

The service is enabled but stopped, and refuses to start.

Likely cause: the new binary cannot read the database schema (status 78, as above), or there is a permission or configuration error.

How to confirm: run `sudo systemctl status levelrail` and check the last few lines of `sudo journalctl -u levelrail -n 50`. If you see "binary older than database", see the previous troubleshooting entry.

Fix: if there is a permission error, check that `APP_DATA_DIR` is readable and writable by the user running the service. If the service file was corrupted during upgrade, run the installer again to rebuild it: `curl -fsSL https://levelrail.com/install.sh | sudo sh -s upgrade`. Then restart with `sudo systemctl restart levelrail`.

Still stuck? Check [Disaster recovery](/disaster-recovery) for how to restore from a backup, or attach `sudo journalctl -u levelrail` (last 100 lines) to an issue on [GitHub](https://github.com/glincker/levelrail/issues).

## Watching an upgrade

While an upgrade runs, a progress bar sits above every page for root users, and the Updates item in the sidebar shows a progress ring. Open **Settings > Updates** for the pipeline view:

- Five stages: Prepare, Validate, Protect, Switch and Confirm. Click a stage to see each step with its start time, duration and log lines.
- The control plane restarts during Switch. The bar and page show "Restarting the control plane, reconnecting..." and settle on the real result when it answers.
- If the new release is not healthy, the failed stage is shown in red with the error and a "Rolled back to vX, your data is intact" notice.
- Animations respect your reduced motion setting.

{/* Screenshot placeholder: pipeline view mid-upgrade, light and dark */}

## Next steps

<CardGroup cols={2}>
<Card title="Installing" href="/installing">

First install and the installer's own upgrade.

</Card>
<Card title="Disaster recovery" href="/disaster-recovery">

Restore from backups after a failed upgrade.

</Card>
</CardGroup>
