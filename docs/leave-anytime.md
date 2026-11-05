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

## Export a whole project

A project exports as one archive with a compose service for every app in it, plus `.env.example`, `volumes.json` and a `README.md`. There are three ways to get it.

- Dashboard: open the project and choose Export. Download saves a `.tar` file.
- CLI: `levelrail-cli apps projects export shop -o ./shop`. Use `-o -` for standard output.
- API: `GET /api/v1/projects/{id}/export`. The project can be given by id or name.

Run it the same way: copy `.env.example` to `.env`, replace every `CHANGE_ME`, then `docker compose up -d`. Restore volume data using `volumes.json` as the list.

The header of `docker-compose.yml` and the README list what did not carry over: the pinned host port, the bind address, registry logins, deploy hooks, labels, the liveness probe, GPU requests and some resource settings. The outbound network allowlist is not enforced by Compose, so add your own firewall rules.

### Secrets in a project export

Secret values stay out by default. To include them, switch on "Include secret values" in the dashboard, or pass `--include-secrets` or `?include_secrets=true`. That adds a `.env` file.

It needs the write-sensitive ability and the same confirmation step as the environment editor, for every app in the project. If one app refuses, the whole export is refused. Each secrets export is written to the audit log before the file is sent. Do not commit the `.env`, and delete the archive when you are done.

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

- Export reads the volume through a short-lived helper container with a read-only mount. For a consistent copy of a busy app, stop it first. If the size cap or time limit cuts an export short, the server aborts the response and the CLI exits non-zero with "export truncated", removing the partial file. The CLI also reads the finished gzip and tar stream to the end before it reports success.
- Import needs the app stopped and the volume empty. `--force` replaces existing files, and only works after you turn on the `volumes.import_overwrite` runtime setting (off by default). The whole archive is staged and checked before the volume is mounted or cleared, so a rejected archive leaves the existing files untouched. Still take a backup first, since a failure while unpacking cannot be undone.
- The app must stay stopped for the whole import. The control plane checks again after the upload and right before the volume is mounted, and refuses to start the app (HTTP 409) while an import into one of its volumes is running. A second import into the same volume gets HTTP 409.
- Imports are staged in a folder under the data directory (`volumes.transfer_spool_dir`, `volume-transfer` by default), not the shared temp directory. Only `volumes.transfer_max_concurrent` imports (1 by default) are staged at once; another gets HTTP 429. Leftover staged files are removed when the control plane starts.
- Import refuses archives with absolute paths, `..` segments, device files, symlinks whose target (followed through other links in the archive) ends outside the volume, and hard links to a file that is not an earlier regular file in the same archive. It drops setuid and setgid bits. The archive is validated end to end before anything is unpacked, so one bad entry anywhere rejects the whole import.
- The volume must belong to the app: if another app holds a record for the same Docker volume name, transfers are refused with HTTP 409, and attaching a volume name that would collide with another app's volume is refused.
- The helper container has no network, drops every capability except the few tar needs to keep file ownership, sets no-new-privileges, uses a read-only root filesystem, and has memory and process limits.
- The size cap is the `volumes.transfer_max_bytes` runtime setting (10 GiB by default, 1 MiB minimum) and the time cap is `volumes.transfer_timeout` (30 minutes by default, 1 minute minimum). Both apply to the next transfer.
- The routes are `GET /api/v1/apps/{name}/volumes/{volume}/export` and `POST /api/v1/apps/{name}/volumes/{volume}/import`. Both need the write-sensitive ability and write a `volume.export` or `volume.import` audit event with the app, volume, entry count and byte count, never file names or contents.
- Volumes that hold the managed mail certificate or key are refused.

## Related

- [Migrate to Levelrail](/migrate-to-levelrail)
- [Environment variables](/environment-variables)
