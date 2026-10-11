---
description: Keep managed databases patched. See available versions with security and end-of-life status, schedule automatic patch and minor upgrades in a maintenance window, and let failed upgrades revert themselves.
---

# Database upgrades

Levelrail tells you which managed databases are behind, which ones miss security fixes, and which run a release past its end of life. It can apply patch and minor releases for you inside a maintenance window. Every upgrade, automatic or started by hand, takes a fresh backup first, checks it, and reverts on its own when the new version does not come up healthy.

Major upgrades are never automatic. For those, use the [guarded Postgres major upgrade](/managing-databases#postgres-major-version-upgrade) or restore a backup into a new database.

<InlineToc default-open />

## What the advisor shows

Open a database and select **Upgrades**, or run:

```bash
levelrail-cli databases upgrades <name>
levelrail-cli databases upgrades            # every database at a glance
```

For each database you get:

- **Current version and release line**, with its upstream end-of-life date and a status: supported, end of life soon (within `APP_DB_EOL_WARN`, default 180 days), or end of life.
- **Available targets**, each marked `patch`, `minor` or `major`: the newest patch of your line, the newest release of each newer minor, and the newest release of each newer major line.
- **Security flags**: a target that fixes a CVE affecting your current version is marked, with the CVE ids.
- **Blockers**, such as a missing backup target or a database on another node.

The data comes from a curated catalog that ships with Levelrail (`internal/dbupgrade/catalog.json`): supported versions, release lines with end-of-life dates, and CVE ids per fixed version. Nothing is fetched while you load the page. A background job also reads the image tag list from Docker Hub once a day and adds newer pinned tags it finds; when the registry is unreachable the last good list is kept. Set `APP_DB_VERSION_REFRESH_INTERVAL=0` to turn that off (for example on an offline host).

A floating tag such as `16` could be any 16.x release, so it is treated as the oldest of its line, and upgrading pins it to an exact release. A pgvector variant (`17-pgvector`) and a non-numeric tag such as `latest` have no targets.

The attention center shows **N databases have security updates available**, plus one item per database past end of life and per database whose last upgrade failed.

## Automatic upgrades

```bash
levelrail-cli databases upgrade-policy <name> --auto patch --window "0 3 * * 0" --duration 2h --timezone Europe/Berlin
levelrail-cli databases upgrade-policy <name> --inherit        # back to the platform default
levelrail-cli databases upgrade-policy --platform --auto patch # the default for every database
```

| Setting | Default | Meaning |
| --- | --- | --- |
| `auto_upgrade` | `off` | `off`, `patch` or `minor`. Never `major`. |
| maintenance window | Sundays 03:00 UTC for 2 hours | A cron start, a duration and a timezone. Daylight saving time follows the wall clock; a start time that does not exist on a spring-forward day is skipped that day. |
| `backup_before` | on | Cannot be turned off. |
| `verify_after` | on | Run the engine health check after the bump. Off only waits for the container to run the new image. |
| `revert_on_failure` | on | Revert automatically when the upgrade fails. |
| `notify` | none | Notification channels (Slack, Discord, email, webhooks and so on) that get a message when a run starts and ends. |

A database without its own policy inherits the platform default (**Settings > Database upgrades**). A run only starts inside the window and only when at least `APP_DB_UPGRADE_MIN_WINDOW_REMAINING` (default 30 minutes) of it is left. A started run always finishes its safety steps, even past the window's end. If an automatic upgrade to a version fails or is reverted, that version is not tried again automatically: start it by hand once you know why, or wait for a newer release.

The database must have a backup target (scheduled backups set one). Without it no upgrade runs.

## Upgrade now

```bash
levelrail-cli databases upgrade-now <name> 16.11 [--confirm <name>]
```

`POST /api/v1/databases/{name}/upgrade-now` with `{"version": "16.11", "confirm": "<name>"}` runs exactly the same steps immediately, outside any window. Majors are refused. A version that is not in the catalog is still accepted when it is a newer patch or minor of the same line.

## What a run does

Each run is stored in `db_upgrade_runs` and moves through these states. The state and the step inside it are saved after every step, so a control plane restart resumes the run where it stopped, and a shutdown in the middle of a step never marks it failed.

```
pending -> backing_up -> upgrading -> verifying -> succeeded
                                          \-> reverted | failed
```

1. **pending**: refuse when a restore or major upgrade of the same database is running.
2. **backing_up**: take a logical backup to the database's backup target, then download it again and verify its checksum, size and format (the same check as `backups verify`). A failure here ends the run with nothing changed.
3. **upgrading**: pull the target image (a missing tag fails here, before any downtime), record the image the database runs now, stop the database and copy its data volume to a snapshot volume, then apply the new version through the same path as `set-version`.
4. **verifying**: wait for the container to run the new image, then connect and run a query. Postgres must not be in recovery and Redis-family engines must report the `master` role. The reported server version must match the target.

When verifying fails within `APP_DB_UPGRADE_HEALTH_TIMEOUT` (default 5 minutes), the run reverts, cheapest path first, and records which path worked:

| Revert path | When | What it does |
| --- | --- | --- |
| `image` | the engine keeps its on-disk format within the line | Start the previous version on the same data. |
| `volume_snapshot` | the old version cannot read the data, or the engine never allows an image revert | Copy the snapshot taken before the bump back over the data volume. Writes since the bump are lost. |
| `backup_restore` | the snapshot is missing or its restore failed | Empty the data volume, start the previous version and restore the verified pre-upgrade backup. |

If every path fails, the run ends `failed` with the snapshot volume and backup id in its reason, so you can restore by hand. With `revert_on_failure` off, a failed run leaves the database on the new version.

The snapshot volume is removed when a run succeeds or reverts. The pre-upgrade backup stays, under its normal retention.

## Engine behaviour

| Engine | Automatic | Revert by image | Notes |
| --- | --- | --- | --- |
| Postgres | patch | yes | Postgres minor releases (16.4 to 16.11) are patches here: same on-disk format, run in place. |
| Redis | patch and minor | yes | A newer minor may write an RDB version an older server cannot load; then the snapshot is restored. |
| MySQL | never | no | A newer server upgrades the data dictionary on first start and downgrades are not supported, even between patches. Use **upgrade now** (snapshot and backup revert) or upgrade by hand. |
| MariaDB | patch | no | Each release series (10.11, 11.4, 11.8) is its own line; moving between series needs `mariadb-upgrade` and stays manual. A failed patch reverts from the snapshot. |
| MongoDB | patch | yes | 7.0 and 8.0 are separate majors; patches keep the featureCompatibilityVersion. |
| ClickHouse | patch | yes | 24.8, 25.3 and 25.8 are separate majors. |
| Dragonfly | patch | yes | Tags such as `v1.27.1`. |
| KeyDB | never | no | Published only as `latest` and architecture-prefixed tags, so there is no version line to follow. |

Upgrades run only for databases on the control plane's own node. A database placed on another node shows that as a blocker.

## Reference

### API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/databases/{name}/upgrades` | `read` |
| `PUT` | `/api/v1/databases/{name}/upgrade-policy` | `write:sensitive` |
| `POST` | `/api/v1/databases/{name}/upgrade-now` | `write:sensitive` |
| `GET` | `/api/v1/databases/upgrade-summary` | `read` |
| `GET` | `/api/v1/settings/database-upgrades` | `read` |
| `PUT` | `/api/v1/settings/database-upgrades` | `root` |

### Environment

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DB_UPGRADE_INTERVAL` | `1m` | How often the controller checks windows and resumes runs. |
| `APP_DB_UPGRADE_HEALTH_TIMEOUT` | `5m` | How long each health wait, and each stop before a snapshot, may take. |
| `APP_DB_UPGRADE_MIN_WINDOW_REMAINING` | `30m` | A run only starts with at least this much window left. |
| `APP_DB_EOL_WARN` | `4320h` (180 days) | When a line counts as end of life soon. |
| `APP_DB_VERSION_REFRESH_INTERVAL` | `24h` | Docker Hub tag refresh; `0` turns it off. |

## Next steps

- [Managing databases](/managing-databases): versions, major upgrades, backups and restores.
- [Backups and storage](/backups-and-storage): set up the backup target every upgrade needs.
