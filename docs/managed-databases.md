---
title: "Managed databases from the command line"
sidebarTitle: "Managed databases"
description: Create a managed database, follow its provisioning steps, plan version upgrades, and rotate the owner password with levelrail-cli.
"og:url": "https://docs.levelrail.com/managed-databases"
---

This page covers the lifecycle commands for a managed database: creating it, following provisioning, choosing versions and upgrade policy, and rotating the owner password. Every command here calls the control plane API, so the dashboard shows the same state.

For backups, restores, access and logins, see [Managing databases](/managing-databases) and [Database access](/database-access). For the upgrade model in depth, see [Database upgrades](/database-upgrades).

## Create a database

```bash
levelrail-cli databases create --name orders-db --engine postgres --version 16 --wait
```

`--name`, `--engine` and `--version` are required. The engines accepted by `--engine` are `postgres`, `redis`, `mysql`, `mongodb`, `mariadb`, `keydb`, `dragonfly`, `clickhouse`, `valkey`, `meilisearch`, `qdrant`, `typesense` and `memcached`.

Version tags depend on the engine. Postgres takes a major such as `16`, or a flavor tag such as `17-pgvector`, `17-timescaledb` or `17-postgis`. Meilisearch and Qdrant take `v`-prefixed tags such as `v1.55`, and Typesense takes tags such as `30.2`. The CLI rejects a version that is not a pinned release tag before it sends the request.

Other flags:

- `--node-id` places the database on a node. Without it, Levelrail places it on the least-loaded registered node, or on the local node when only one exists.
- `--existing-volume reuse|discard` decides what happens to a data volume a deleted database of the same name left behind. See [Leftover volumes](#leftover-volumes).
- `--wait` prints each provisioning step until the database is ready or fails.
- `--interactive` (or `-i`) runs a prompt-by-prompt wizard instead of flags.
- `--json` prints the created database as JSON and nothing else.

Without `--wait`, the command returns once the request is accepted. Use `levelrail-cli databases status orders-db` to read the current reconcile conditions.

## Provisioning steps

Creation runs ten steps in this order. The steps are the same in the API and the CLI.

| Step | What happens |
| --- | --- |
| `accepted` | The request is recorded. |
| `docker_check` | The container runtime answers. |
| `pull_image` | The engine image is pulled, or reported as already present. |
| `create_volume` | The data volume is created. |
| `credentials_and_tls` | The owner password and TLS material are prepared. |
| `create_container` | The container is created. |
| `start` | The container starts. |
| `wait_ready` | The engine accepts connections. |
| `apply_settings` | The database settings are applied, and the step detail gives a summary. |
| `ready` | The database is ready for connections. |

With `--wait`, each line is a step mark, the step name and its detail:

```text
[x] accepted: request accepted
[x] docker_check
[>] pull_image: pulling <image>
[x] pull_image: image pulled
```

The marks mean: `[ ]` pending, `[>]` running, `[x]` done, `[-]` skipped and `[!]` failed. A failure prints `failed at <step>: <reason>`, then a `fix:` line when Levelrail knows a recovery.

The wait stops after `APP_CLI_PROVISION_WAIT_TIMEOUT` (default `15m`) and exits with code `2`. The timeout only stops the CLI from waiting. It does not cancel the attempt. The CLI polls every `APP_CLI_PROVISION_POLL_INTERVAL` (default `1s`).

The control plane keeps the last `APP_DB_PROVISION_KEEP_ATTEMPTS` attempts per database (default `5`). Progress events for one step are written at most once per `APP_DB_PROVISION_EVENT_INTERVAL` (default `500ms`), so the history stays short during a long image pull.

### Failure fixes

| Fix label | When it is offered |
| --- | --- |
| Remove leftover volume and retry | A data volume from a deleted database of the same name is still there. Asks for confirmation. |
| Re-check Docker | The Docker daemon or its guard socket is not responding. |
| Free up disk space | The engine reported no space left on the device. |
| Retry | An image pull failed, or a port or name conflict was reported. |

When the API reports a container crash after start, it also returns a cause. The cause is one of `permission`, `out_of_memory`, `port_in_use`, `bad_volume` or `bad_config`.

### Leftover volumes

If a deleted database left its data volume behind, creation stops at `create_volume` until you choose:

```bash
levelrail-cli databases create --name orders-db --engine postgres --version 16 --existing-volume reuse --wait
levelrail-cli databases create --name orders-db --engine postgres --version 16 --existing-volume discard --wait
```

`reuse` attaches the old data. `discard` deletes the old volume and starts empty. The choice is stored with the database.

## Versions and upgrades

Check what an engine offers and what a database can move to:

```bash
levelrail-cli databases versions postgres
levelrail-cli databases versions postgres --refresh
levelrail-cli databases upgrades orders-db
levelrail-cli databases upgrades
```

`--refresh` looks the engine up live now. Without a name, `upgrades` lists every database with its security and end-of-life status.

### Minor and patch changes in place

`set-version` changes the image on the same data volume. It is for minor and patch changes, and the engine check refuses switches it cannot make safely:

```bash
levelrail-cli databases set-version orders-db 16.11
```

Use `upgrade-now` when you want the upgrade to take a backup, verify it, upgrade, run a health check, and revert on failure:

```bash
levelrail-cli databases upgrade-now orders-db 16.11
levelrail-cli databases upgrade-now orders-db 16.11 --confirm orders-db
```

`upgrade-now` needs a backup target on the database, a running database, and a confirmation. Without `--confirm NAME`, it asks you to type the database name.

### Upgrade policy

```bash
levelrail-cli databases upgrade-policy orders-db --auto patch --window "0 3 * * 0" --duration 2h --timezone Europe/Berlin
levelrail-cli databases upgrade-policy --platform --auto off
```

- `--auto` is `off`, `patch` or `minor`. Major upgrades are never automatic.
- A maintenance window (`--window` as cron, `--duration`) is required when `--auto` is not `off`.
- A backup before every upgrade cannot be turned off.
- `--verify` (default on) runs the engine health check after the change. `--revert` (default on) reverts a failed upgrade.
- The engine catalog sets the ceiling for each engine. For example, Postgres and MariaDB stop at patch, Redis at minor, and MySQL is manual only.

An automatic run starts only while the window is open and at least `APP_DB_UPGRADE_MIN_WINDOW_REMAINING` (default `30m`) is left. If an automatic run to a version fails or is reverted, Levelrail does not retry that version every window. You decide the next step.

### What a run does

A run moves through these phases, and `databases upgrades <name>` shows the state of each one:

1. `backing_up`: take a logical backup, then verify it.
2. `upgrading`: check the target image and the version change. If revert is on, take a volume snapshot. Then change the version.
3. `verifying`: wait for the engine to report healthy.
4. `succeeded`, `reverted` or `failed`.

A failed run reverts in this order, skipping steps that do not apply: the old image on the same data (not for MySQL and MariaDB, where the catalog does not allow it), then the volume snapshot, then the logical backup. If every step fails, the run says where the pre-upgrade data is kept so you can restore it by hand.

The timing knobs (`APP_DB_UPGRADE_INTERVAL`, `APP_DB_UPGRADE_HEALTH_TIMEOUT`, `APP_DB_EOL_WARN`) are listed in [Database upgrades](/database-upgrades#environment).

### Major upgrades (Postgres)

Major versions are never automatic. Postgres has a guarded dump-and-restore path:

```bash
levelrail-cli databases major-upgrade orders-db --version 17
levelrail-cli databases major-upgrades orders-db
levelrail-cli databases major-upgrade-rollback orders-db <upgrade-id>
levelrail-cli databases major-upgrade-discard orders-db <upgrade-id>
```

- The database is offline for the duration.
- The old data is copied to a snapshot volume first, and the dump is taken from that copy. A failure after the live volume is wiped restores the snapshot and the old version.
- After success, the snapshot is kept so `major-upgrade-rollback` can undo the upgrade. Rolling back loses writes made since the upgrade. `major-upgrade-discard` deletes the snapshot and frees the disk.
- It is refused while point-in-time restore is enabled, and for downgrades.
- It needs free disk for the snapshot (the size of the data volume) plus the dump file.

For other engines, the CLI points you to restoring a backup into a new database, for example with `databases branch create`.

## Rotate the owner password

```bash
levelrail-cli databases rotate-password orders-db --dry-run
levelrail-cli databases rotate-password orders-db
levelrail-cli databases rotate-password orders-db --status
levelrail-cli databases rotate-password orders-db --every-days 90
levelrail-cli databases rotate-password orders-db --every-days 0
```

`--dry-run` lists the apps that would restart and changes nothing. A real rotation asks for your password and authenticator code.

A rotation does this:

1. Generates a new owner password and stages it.
2. Sets it in the engine, using the password in force now.
3. Logs in as the owner with the new password. This is a real login, not a status check.
4. Switches the stored credential and restarts the linked apps.

If the engine refuses the new password, the old one is restored and the rotation is marked failed. If the restore cannot be confirmed, the rotation stays pending and the next scheduled pass settles it. `--status` shows the last rotation, its age and the schedule.

Limits:

- Supported engines are `postgres`, `mysql`, `mariadb` and `mongodb`. Other engines return "this engine has no owner password to rotate".
- Only the owner password changes. Other logins are managed with `databases users`.
- `--every-days N` sets an automatic schedule from 0 to 3650 days. `0` turns it off. Setting the schedule needs the root ability, because it changes the database policy.
- If a previous rotation is still settling, a new one is refused with a conflict. Try again shortly.

The scheduler runs at boot and then every `APP_DATABASE_PASSWORD_ROTATION_INTERVAL_SECONDS` (default `3600`). Each pass settles interrupted rotations and rotates at most five due databases.

## Not yet

- A CLI command that shows one provisioning attempt on its own. Today use `databases create --wait`, or `GET /api/v1/databases/{name}/provision` in the API.
- A CLI command to discard a pending provisioning attempt. The API has `POST /api/v1/databases/{name}/provision/discard`, and the CLI has no matching subcommand.
- Guarded major upgrades for engines other than Postgres.
- Owner password rotation for engines beyond the four listed above.
