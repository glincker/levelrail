## Known failures

| Template | Cause | Status |
| --- | --- | --- |
| `woodpecker` | The agent needs a container backend, which means mounting the Docker socket into the agent. The template does not mount it, so the agent exits with "can't detect an available backend engine". The server starts and passes its own check. | Left as is: a CI runner with socket access is host-level exec, so adding it needs an explicit owner decision. |
| `mattermost` | The pinned image publishes no linux/arm64 manifest, so the pull fails on Apple silicon and arm64 VPS nodes. It passes on amd64 hosts. | Harness limit on this arm64 host, not a template bug. |
| `cyberchef`, `flame`, `mailhog`, `snapdrop` | The pinned images publish only linux/amd64 (`exec format error` on arm64). | Harness limit on this arm64 host, not a template bug. Run the smoke on an amd64 host to confirm. |
| `redlib` | The server never listens: its startup OAuth handshake with Reddit returns 401 from this network. | Upstream or network block, not a template bug. |

Fixed in the last pass (template, not harness): `gotify`, `vaultwarden`, `pgadmin`, `jellyfin`, `nextcloud`, `open-webui`, `immich` and `penpot` probed with a binary their image does not ship, so each healthcheck now tries `curl` and falls back to `wget` (the platform still turns it into an HTTP probe); `actual-budget` and `authentik` use the image's own health tool; `changedetection` probes with Python; `memos` tries `wget` first; `umami`, `directus` and `outline` start their database first (healthcheck plus `depends_on`); `directus` gained the missing `DB_PORT`; `outline` sets `PGSSLMODE=disable` for its bundled Postgres; `penpot` frontend points at the real `backend` and `exporter` service names. The harness now resolves `SERVICE_FQDN_*` to a full URL like the platform does (it broke `ghost`).

Fixed in the wave N pass: `valkey` interpolated its password from the host shell (blank), so the command and healthcheck now use the generated token directly; `2fauth` had an `APP_KEY` of the wrong length (500 on every request); `wakapi` lacked `WAKAPI_DB_PORT` and waited on a database that was not ready; `kopia` set a `command` that the image entrypoint swallowed, so it now overrides `entrypoint`, and its healthcheck authenticates; `glance` exited without a config file, so it writes a minimal one on first start; `privatebin`, `rabbitmq` and `navidrome` probed with `curl`, which the image lacks, so they fall back to `wget`; `triliumnext` probes with Node. `shiori`, `vikunja` and `loki` ship no shell, so like `portainer` the harness only checks that they stay running.

`portainer` (distroless, no shell), `keycloak` and `typesense` (no curl or wget) cannot run an in-container check under plain `docker compose`. The platform probes them over HTTP from outside, so their templates keep the HTTP healthcheck, and the harness only verifies that they stay running.

`paperless-ngx` failed once on a first cold start and passed on every rerun; treat it as slow to first boot, not broken.

## Upgrade paths, top 20

Applies to all of them: the platform keeps the previous image pinned, so a bad upgrade rolls back, but a database migration run by the new version does not roll back with the image. Take the backup first, then move the tag.

- **ghost**: back up the `ghost_db_data` MySQL volume and `ghost_data` (themes, images). Move the `ghost:5` tag within 5.x freely; Ghost runs its own migrations on boot. Stay on MySQL 8.x, Ghost does not support other databases.
- **wordpress**: back up `wordpress_db_data` and `wordpress_data` (plugins, uploads, wp-config). Bump the `6.6-apache` tag one minor at a time, update plugins afterwards, and run the database upgrade prompt at `/wp-admin/upgrade.php` if shown.
- **nextcloud**: back up `nextcloud_data`. Nextcloud only upgrades one major version at a time (29 to 30, never 29 to 31), and `occ upgrade` runs on boot. Check that your apps support the next major before moving the tag.
- **grafana**: back up `grafana_data` (dashboards, users, SQLite database). Minor bumps are safe; read the breaking changes list before a major, and expect the database to migrate forward on first boot.
- **uptime-kuma**: back up `data` (SQLite and uploads). The 1.23.x line is a straight tag bump. Version 2 migrates the database to a new layout that cannot be reversed, so back up and upgrade deliberately.
- **n8n**: back up `n8n_data`, which holds the encryption key as well as the database. Losing that key makes stored credentials unreadable. Read the release notes between versions for removed nodes before moving the tag.
- **plausible**: back up both `plausible_db_data` (Postgres) and `plausible_clickhouse_data` (events). Move the `community-edition` tag one release at a time and keep the ClickHouse version unless the release notes ask for a bump.
- **metabase**: back up `metabase_data` (the embedded H2 application database). Metabase migrates it forward only, and older versions cannot open it again, so copy the volume before a tag change. Move to Postgres for production use.
- **vaultwarden**: back up `vaultwarden_data` (database, attachments, RSA keys) and keep that copy offline. Tag bumps are safe and frequent, since the web vault must match the server.
- **gitea**: back up `gitea_db_data` and `gitea_data` (repositories, config). Gitea migrates its database on boot and does not support downgrades. Move one minor version at a time and read the breaking changes list.
- **minio**: back up the `minio_data` bucket volume. Pin to a specific release tag, never `latest`, and note the image moved to the AIStor distribution, so check its license terms before the next bump.
- **portainer**: back up `portainer_data`. Move the tag within the same major; Portainer updates its BoltDB schema on boot, which is one way, so keep the copy.
- **nocodb**: back up `nocodb_data` (SQLite metadata and attachments). Tags follow a year and month scheme, and metadata migrates forward on boot, so step through releases instead of jumping many at once.
- **directus**: back up `directus_db_data` (Postgres) and `directus_uploads`. Run the schema migration step that Directus performs on boot, keep to the 11.x line, and snapshot the schema before a minor bump.
- **strapi**: back up `strapi_db_data` and `strapi_uploads`. This image is a third-party production build, so confirm the Strapi major it ships matches your content types before moving the tag.
- **adminer**: stateless, nothing to back up. Bump the `5` tag any time.
- **pgadmin**: back up `pgadmin_data` (saved servers, users). Move the `8.x` tag within the major; pgAdmin migrates its SQLite config on boot.
- **umami**: back up `umami_db_data` (Postgres). Umami applies Prisma migrations on boot, forward only. Keep the `postgresql-` variant of the tag and bump one release at a time.
- **penpot**: back up `penpot_db_data` and `penpot_assets`. The frontend, backend and exporter tags must move together to the same version, and Penpot migrates files lazily after the database migration.
- **jellyfin**: back up `jellyfin_config` (library database, users). Media in `jellyfin_media` is not touched. Move one minor at a time; the database migrates forward on boot and older servers cannot read it.
