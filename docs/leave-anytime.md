---
title: "Leave anytime"
sidebarTitle: "Leave anytime"
description: Export an app as plain Docker Compose plus an env template, so moving off Levelrail never needs Levelrail.
"og:url": "https://docs.levelrail.com/leave-anytime"
---

Your apps are never locked in. One command writes an app as ordinary Docker Compose files that run on any host with Docker.

```bash
levelrail-cli apps export web --format compose --out ./web-export
cd web-export
cp .env.example .env     # fill in every CHANGE_ME
docker compose up -d
```

`--out -` streams the tar archive to stdout instead. The API route is `GET /api/v1/apps/{name}/export?format=compose`. It needs the read ability on the app and writes an `app.export` audit event that names the app and service count, never a value.

For a multi-service app, the name of any member or of the app exports every service together.

## What you get

| File | Content |
| --- | --- |
| `docker-compose.yml` | Image, command, entrypoint, ports, named volumes, bind mounts, healthcheck, `depends_on` with `service_started` or `service_healthy`, restart policy, memory and CPU limits. Domains are kept as `x-domains`, which Docker ignores, so you can map them in your own proxy. |
| `.env.example` | Every env key. Plain values are written. Secrets are `CHANGE_ME` with a comment. |
| `README.md` | The exact steps, volume restore, database dump and restore commands, and a domain note. |

Databases that are services of the app keep the same engine and version tag. The README gives `pg_dump`, `mysqldump` or `mongodump` commands for them.

The output is sorted and byte-stable, so you can commit it to git and diff it.

## What is not exported

- Secret values. Anything stored as a secret, bound from a managed database or an external vault, and any key or value that looks like a credential (names with PASSWORD, TOKEN, KEY and similar, or URLs with a password), is replaced by `CHANGE_ME`. Re-enter them or restore them from your own backup.
- Backup data and volume contents. Download them with the backup commands and restore as the README describes.
- TLS certificates. Use a reverse proxy with automatic certificates on the new host.
- Managed config file contents and private registry logins.
- Levelrail features: previews, auto-sleep, deploy hooks, zero-downtime cutover and rollback.

## Move to plain Compose

1. Export the app and review `docker-compose.yml`.
2. Copy volume data and take database dumps on the old host (see the README in the export).
3. Create `.env` on the new host and fill in the secrets.
4. `docker compose up -d`, restore data, check the app.
5. Point DNS at the new host.

To go the other way, see [Migrate from Docker Compose](/migrate-from-docker-compose).

## Move volume data

Volume contents travel as a `tar.gz`, in or out of Levelrail.

```bash
levelrail volumes export web data --out web-data.tar.gz
levelrail apps stop web
levelrail volumes import web data --in web-data.tar.gz
```

- Export reads the volume through a short-lived helper container with a read-only mount. For a consistent copy of a busy app, stop it first.
- Import needs the app stopped and the volume empty. `--force` replaces existing files, and only works after you turn on the `volumes.import_overwrite` runtime setting (off by default). Take a backup first.
- Import refuses archives with absolute paths, `..` segments, links that leave the volume, or device files, and drops setuid and setgid bits. Nothing is unpacked from a rejected archive, though a bad entry later in the stream stops the import part way.
- The size cap is the `volumes.transfer_max_bytes` runtime setting (10 GiB by default, 1 MiB minimum) and the time cap is `volumes.transfer_timeout` (30 minutes by default, 1 minute minimum). Both apply to the next transfer.
- The routes are `GET /api/v1/apps/{name}/volumes/{volume}/export` and `POST /api/v1/apps/{name}/volumes/{volume}/import`. Both need the write-sensitive ability and write a `volume.export` or `volume.import` audit event with the app, volume, entry count and byte count, never file names or contents.
- Volumes that hold the managed mail certificate or key are refused.
