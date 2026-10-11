---
description: "Every version the control plane has run, recorded at boot, and how upgrades are acknowledged."
---

# Upgrade history

Settings > Updates shows every version the control plane has run. Upgrades are never silent: each change waits for someone to acknowledge it.

## How it is recorded

The control plane records history itself, at boot. It compares the running version with the last version it recorded and, when they differ, appends one row. That works however the binary was replaced: the installer, a package manager, a cron job or `rollback`.

- A restart, a crash loop or a double start of the same version adds nothing.
- A row has a kind: `installed`, `adopted` (history began on an existing install), `upgraded`, `rolled_back`, `rebuilt`, `development` or `changed`.
- A pseudo-version or `dev` build is a development build. It gets no release or compare link.
- Each row keeps the schema version before and after, the pre-migration backup name, the initiator and method when known, and a snapshot of the release notes fetched best effort at that time (offline is fine; the row says the notes were unavailable).
- Rows are append-only. The database refuses updates and deletes except the one-time release-notes fill and the acknowledgement.

## Self-upgrade attempts

Upgrades run by the safe self-update (Settings > Updates > Upgrade this server, `levelrail-cli upgrade --apply`, `levelrail self-upgrade`) are also recorded as attempts with a step timeline, including ones that were refused or rolled back, which never reach this history because the old version kept running. `levelrail-cli upgrade --attempts` lists them, API `GET /api/v1/updates/self-upgrade/attempts`. The version transition itself still appears here with method `self-upgrade`. See [Upgrade safely](upgrade-safely.md).

## Who did it

`install.sh` and `sudo <binary> rollback` leave a small `upgrade-context.json` in the data directory (user, method, channel, backup name, time). The next boot consumes it exactly once and deletes it. A marker for a different version is ignored, and one older than 24 hours is discarded. With no marker the initiator is `unknown`, and the page says so plainly.

The marker holds no secrets and is written with owner-only permissions.

### Swapping the binary by hand

A manual swap (a package manager, your own CI, `scp`) leaves no marker, so the initiator shows as `unknown`. Record one with `upgrade-note` after replacing the binary and before starting the service:

```bash
sudo systemctl stop levelrail
sudo install -m 0755 ./levelrail-linux-amd64 /usr/local/bin/levelrail
sudo levelrail upgrade-note --by "$USER" --method manual --reason "hotfix build"
sudo systemctl start levelrail
```

`--by` is required. `--method` is `manual` (default), `package` or `ci`, `--reason` is optional free text (up to 120 characters, kept in the audit entry), and `--data-dir` defaults to `APP_DATA_DIR` or the standard data directory. The note is written for the version of the binary that runs the command, so run it with the new binary. It follows the same rules as the other markers: consumed once on the next boot, ignored by a different version, discarded after 24 hours.

## Acknowledging

An unacknowledged change shows a ribbon with an Acknowledge button. The first acknowledgement wins and records who and when. Every recorded transition and every acknowledgement is written to the audit log (actions `upgrade_history.recorded` and `upgrade_history.acknowledged`).

```
levelrail-cli upgrade --history [--json]
levelrail-cli upgrade --ack <id>
```

API: `GET /api/v1/updates/history`, `POST /api/v1/updates/history/{id}/ack`. The MCP tool `list_upgrade_history` is read-only; no tool can acknowledge or change history.

## Node agents

Each node's agent version change is recorded when the node reports it (the existing heartbeat data, no new protocol) and listed under the history.

## Attention center

`store.DB.ListUnacknowledgedUpgrades` returns every unacknowledged transition, newest first. It is the query an attention item should read.
