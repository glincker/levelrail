---
title: "Template status"
description: Measured pass rate of the one-click template catalog from the local smoke run
"og:url": "https://docs.levelrail.com/templates-status"
---

Measured by the catalog smoke script (scripts/catalog-smoke.sh): each template's compose file is brought up with `docker compose up --wait`, checked for running and healthy containers, then torn down with its volumes. Host ports are dropped so runs never collide. Regenerate with `scripts/catalog-smoke.sh -r` (all of the top 50) or pass `-i id1,id2` for a subset. A fail with an unhealthy container means the template's own healthcheck never passed on this host, so check that healthcheck and the image first.

Per-template Arm support is on the [Arm64 compatibility](/arm-compatibility) page.

Last run: 2026-10-25, plus eight templates booted by hand on 2026-10-27 under the platform hardening flags (seconds not measured). **101 of 108 passed (94%).** Templates that need operator configuration are skipped and listed in the run output, not counted.

| Template | Result | Seconds | Note |
| --- | --- | --- | --- |
| `2fauth` | pass | 7 |  |
| `actual-budget` | pass | 11 |  |
| `adminer` | pass | 33 |  |
| `alertmanager` | pass | 11 |  |
| `apprise-api` | pass | 10 |  |
| `appsmith` | pass | 65 |  |
| `archivebox` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `authelia` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `authentik` | pass | 63 |  |
| `backrest` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `bentopdf` | pass | 6 |  |
| `bookstack` | pass | 23 |  |
| `bytestash` | pass | 10 |  |
| `changedetection` | pass | 17 |  |
| `consul` | pass | 7 |  |
| `couchdb` | pass | 7 |  |
| `crawl4ai` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `cyberchef` | fail | 4 | image is amd64 only (exec format error), host limit on arm64 |
| `dashy` | pass | 11 |  |
| `directus` | pass | 44 |  |
| `docker-registry` | pass | 12 |  |
| `etcd` | pass | 8 |  |
| `excalidraw` | pass | 33 |  |
| `filebrowser` | pass | 8 |  |
| `filebrowser-quantum` | pass | 26 |  |
| `firefly-iii` | pass | 28 |  |
| `flame` | fail | 5 | image is amd64 only (exec format error), host limit on arm64 |
| `flatnotes` | pass | 9 |  |
| `forgejo` | pass | 11 |  |
| `ghost` | pass | 20 |  |
| `gitea` | pass | 35 |  |
| `glance` | pass | 31 |  |
| `goatcounter` | pass | 13 |  |
| `gotenberg` | pass | 46 |  |
| `gotify` | pass | 6 |  |
| `grafana` | pass | 35 |  |
| `headscale` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `heimdall` | pass | 10 |  |
| `homepage` | pass | 10 |  |
| `homer` | pass | 7 |  |
| `immich` | pass | 47 |  |
| `influxdb` | pass | 34 |  |
| `it-tools` | pass | 33 |  |
| `jellyfin` | pass | 46 |  |
| `kavita` | pass | 17 |  |
| `keycloak` | pass | 35 |  |
| `kopia` | pass | 7 |  |
| `librespeed` | pass | 13 |  |
| `linkding` | pass | 16 |  |
| `listmonk` | pass | 39 |  |
| `lldap` | pass | 8 |  |
| `loki` | pass | 1 |  |
| `mailhog` | fail | 5 | image is amd64 only (exec format error), host limit on arm64 |
| `mailpit` | pass | 12 |  |
| `mattermost` | pull_failed | 0 | no linux/arm64 image for the pinned tag (host limit) |
| `mealie` | pass | 16 |  |
| `meilisearch` | pass | 33 |  |
| `memcached` | pass | 12 |  |
| `memos` | pass | 11 |  |
| `metabase` | pass | 37 |  |
| `metube` | pass | 13 |  |
| `minio` | pass | 37 |  |
| `mosquitto` | pass | 7 |  |
| `n8n` | pass | 41 |  |
| `nats` | pass | 8 |  |
| `navidrome` | pass | 12 |  |
| `nextcloud` | pass | 40 |  |
| `nocodb` | pass | 14 |  |
| `ntfy` | pass | 32 |  |
| `open-notebook` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `open-webui` | pass | 82 |  |
| `outline` | pass | 47 |  |
| `pairdrop` | pass | 14 |  |
| `paperless-ngx` | pass | 19 |  |
| `penpot` | pass | 52 |  |
| `pgadmin` | pass | 16 |  |
| `phpmyadmin` | pass | 18 |  |
| `plausible` | pass | 41 |  |
| `pocket-id` | pass | 8 |  |
| `pocketbase` | pass | 13 |  |
| `portainer` | pass | 4 |  |
| `privatebin` | pass | 12 |  |
| `prometheus` | pass | 34 |  |
| `radicale` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `rabbitmq` | pass | 12 |  |
| `readeck` | pass | 17 |  |
| `redlib` | fail | 33 | upstream OAuth to Reddit returns 401 so the server never listens |
| `rocketchat` | pass | 43 |  |
| `rsshub` | pass | 0 | booted with cap-drop ALL, no-new-privileges, pids limit; service proof checked |
| `rustfs` | pass | 26 |  |
| `searxng` | pass | 34 |  |
| `shiori` | pass | 1 |  |
| `silverbullet` | pass | 14 |  |
| `snapdrop` | pull_failed | 1 | no linux/arm64 image (host limit) |
| `sonarqube` | pass | 25 |  |
| `strapi` | pass | 81 |  |
| `syncthing` | pass | 34 |  |
| `tautulli` | pass | 10 |  |
| `triliumnext` | pass | 7 |  |
| `typesense` | pass | 36 |  |
| `umami` | pass | 43 |  |
| `uptime-kuma` | pass | 16 |  |
| `valkey` | pass | 7 |  |
| `vaultwarden` | pass | 31 |  |
| `verdaccio` | pass | 10 |  |
| `vikunja` | pass | 8 |  |
| `wakapi` | pass | 17 |  |
| `wallos` | pass | 9 |  |
| `woodpecker` | fail | 32 | agent needs a Docker socket backend, left for owner decision |
| `wordpress` | pass | 13 |  |

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

{/* BEGIN GENERATED ARM64 TABLE */}
## Arm64 support

Generated from the registry scan in `internal/catalog/platforms.json` (`scripts/scan-catalog-platforms`), not from the smoke run. A template is `yes` only when every image it uses lists `linux/arm64` in its manifest. `not checked` means unknown, not unsupported: the scan could not read a manifest or the image tag is set at install time.

Last scan: 2026-11-02T03:14:45Z. **393 yes, 15 amd64 only, 32 not checked** of 440 templates.

| Template | arm64 | Verified platforms |
| --- | --- | --- |
| `2fauth` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `ackee` | yes | linux/amd64, linux/arm64 |
| `activepieces` | yes | linux/amd64, linux/arm64 |
| `actual-budget` | yes | linux/amd64, linux/arm64 |
| `actualbudget` | yes | linux/amd64, linux/arm64 |
| `adguard-home` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le |
| `adminer` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `affine` | yes | linux/amd64, linux/arm64 |
| `alertmanager` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `answer` | yes | linux/amd64, linux/arm64 |
| `anythingllm` | yes | linux/amd64, linux/arm64 |
| `appflowy` | yes | linux/amd64, linux/arm64 |
| `apprise-api` | yes | linux/amd64, linux/arm64 |
| `appsmith` | yes | linux/amd64, linux/arm64 |
| `appwrite` | yes | linux/amd64, linux/arm64 |
| `archivebox` | yes | linux/amd64, linux/arm64 |
| `athens` | not checked | - |
| `audiobookshelf` | yes | linux/amd64, linux/arm64 |
| `authelia` | yes | linux/amd64, linux/arm, linux/arm64 |
| `authentik` | yes | linux/amd64, linux/arm64 |
| `autobrr` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x, windows/amd64 |
| `babybuddy` | yes | linux/amd64, linux/arm64 |
| `backrest` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `baserow` | yes | linux/amd64, linux/arm64 |
| `bazarr` | yes | linux/amd64, linux/arm64 |
| `bentopdf` | yes | linux/amd64, linux/arm64 |
| `beszel` | yes | linux/amd64, linux/arm, linux/arm64 |
| `blackbox-exporter` | not checked | - |
| `bookstack` | yes | linux/amd64, linux/arm64 |
| `browserless` | yes | linux/amd64, linux/arm64 |
| `budge` | yes | linux/amd64, linux/arm64 |
| `budibase` | yes | linux/arm64 |
| `bugsink` | yes | linux/amd64, linux/arm64 |
| `bytebase` | yes | linux/amd64, linux/arm64 |
| `bytestash` | yes | linux/amd64, linux/arm, linux/arm64 |
| `calcom` | amd64 only | linux/amd64 |
| `calibre-web` | yes | linux/amd64, linux/arm64 |
| `campfire` | yes | linux/amd64, linux/arm64 |
| `cap-captcha` | yes | linux/amd64, linux/arm64 |
| `castopod` | yes | linux/amd64, linux/arm64 |
| `changedetection` | yes | linux/amd64, linux/arm, linux/arm64 |
| `chatwoot` | yes | linux/amd64, linux/arm64 |
| `checkmate` | yes | linux/amd64, linux/arm64 |
| `chibisafe` | yes | linux/amd64, linux/arm64 |
| `chroma` | yes | linux/amd64, linux/arm64 |
| `cleanuparr` | yes | linux/amd64, linux/arm64 |
| `clickhouse` | yes | linux/amd64, linux/arm64 |
| `cloudbeaver` | yes | linux/amd64, linux/arm64 |
| `cloudflare-ddns` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `cloudreve` | yes | linux/amd64, linux/arm64 |
| `cockpit-cms` | yes | linux/amd64, linux/arm64 |
| `code-server` | yes | linux/amd64, linux/arm64 |
| `comfyui` | amd64 only | linux/amd64 |
| `commafeed` | not checked | - |
| `consul` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `convertx` | yes | linux/amd64, linux/arm64 |
| `convertx-app` | yes | linux/amd64, linux/arm64 |
| `copyparty` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `couchdb` | yes | linux/amd64, linux/arm64, linux/s390x |
| `crawl4ai` | yes | linux/amd64, linux/arm64 |
| `cryptgeon` | yes | linux/amd64, linux/arm64 |
| `cyberchef` | amd64 only | linux/amd64 |
| `dashy` | yes | linux/amd64, linux/arm64 |
| `databasus` | yes | linux/amd64, linux/arm64 |
| `deluge` | yes | linux/amd64, linux/arm64 |
| `directus` | yes | linux/amd64, linux/arm64 |
| `diun` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le |
| `docker-registry` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `docker-registry-auth` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `docling-serve` | yes | linux/amd64, linux/arm64 |
| `docmost` | yes | linux/amd64, linux/arm64 |
| `documenso` | yes | linux/amd64, linux/arm64 |
| `docuseal` | yes | linux/amd64, linux/arm64 |
| `dokuwiki` | yes | linux/amd64, linux/arm64 |
| `dolibarr` | yes | linux/amd64, linux/arm64 |
| `drawio` | yes | linux/amd64, linux/arm64 |
| `drupal` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `duplicati` | yes | linux/amd64, linux/arm64 |
| `easyappointments` | yes | linux/amd64, linux/arm64 |
| `elasticsearch` | yes | linux/amd64, linux/arm64 |
| `elasticsearch-kibana` | yes | linux/amd64, linux/arm64 |
| `element-web` | yes | linux/amd64, linux/arm64 |
| `emby` | yes | linux/amd64, linux/arm64 |
| `embystat` | yes | linux/amd64, linux/arm64 |
| `espocrm` | yes | linux/amd64, linux/arm64 |
| `etcd` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/s390x |
| `etherpad` | yes | linux/amd64, linux/arm64 |
| `excalidraw` | yes | linux/amd64, linux/arm, linux/arm64 |
| `factorio` | yes | linux/amd64, linux/arm64 |
| `fastapi-postgres-starter` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `ferretdb` | not checked | - |
| `fider` | yes | linux/amd64, linux/arm64 |
| `filebrowser` | yes | linux/amd64, linux/arm, linux/arm64 |
| `filebrowser-quantum` | yes | linux/amd64, linux/arm64 |
| `firefly-iii` | yes | linux/amd64, linux/arm64 |
| `firefox-browser` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `flagsmith` | yes | linux/amd64, linux/arm64 |
| `flame` | amd64 only | linux/amd64 |
| `flaresolverr` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `flatnotes` | yes | linux/amd64, linux/arm64 |
| `flipt` | yes | linux/amd64, linux/arm64 |
| `flowise` | yes | linux/amd64, linux/arm64 |
| `flowise-postgres` | yes | linux/amd64, linux/arm64 |
| `focalboard` | amd64 only | linux/amd64 |
| `forgejo` | yes | linux/amd64, linux/arm64 |
| `forgejo-postgres` | yes | linux/amd64, linux/arm64 |
| `formbricks` | yes | linux/amd64, linux/arm64 |
| `freescout` | yes | linux/amd64, linux/arm64 |
| `freshrss` | yes | linux/amd64, linux/arm64 |
| `gatus` | yes | linux/amd64, linux/arm, linux/arm64 |
| `ghost` | yes | linux/amd64, linux/arm64 |
| `ghostfolio` | yes | linux/amd64, linux/arm, linux/arm64 |
| `gitea` | yes | linux/amd64, linux/arm64 |
| `gitea-postgres` | yes | linux/amd64, linux/arm64 |
| `gitlab` | amd64 only | linux/amd64 |
| `gitlab-ce` | yes | linux/amd64, linux/arm64 |
| `gitness` | yes | linux/amd64, linux/arm64 |
| `glance` | yes | linux/amd64, linux/arm, linux/arm64 |
| `glances` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `glitchtip` | yes | linux/amd64, linux/arm64 |
| `glpi` | yes | linux/amd64, linux/arm64 |
| `goatcounter` | yes | linux/amd64, linux/arm64 |
| `gogs` | yes | linux/amd64, linux/arm, linux/arm64 |
| `gotenberg` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `gotify` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/riscv64 |
| `grafana` | yes | linux/amd64, linux/arm, linux/arm64 |
| `grafana-alloy` | not checked | - |
| `grafana-postgres` | yes | linux/amd64, linux/arm, linux/arm64 |
| `gramps-web` | yes | linux/amd64, linux/arm, linux/arm64 |
| `grav` | not checked | - |
| `grimmory` | yes | linux/amd64, linux/arm64 |
| `grist` | yes | linux/amd64, linux/arm64 |
| `grocy` | yes | linux/amd64, linux/arm64 |
| `hasura` | yes | linux/amd64, linux/arm64 |
| `headscale` | yes | linux/amd64, linux/arm64 |
| `healthchecks` | yes | linux/amd64, linux/arm, linux/arm64 |
| `hedgedoc` | yes | linux/amd64, linux/arm64 |
| `heimdall` | yes | linux/amd64, linux/arm64 |
| `heyform` | yes | linux/amd64, linux/arm64 |
| `homarr` | yes | linux/amd64, linux/arm, linux/arm64 |
| `home-assistant` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `homebox` | yes | linux/amd64, linux/arm, linux/arm64 |
| `homepage` | yes | linux/amd64, linux/arm, linux/arm64 |
| `homer` | yes | linux/amd64, linux/arm, linux/arm64 |
| `hoppscotch` | yes | linux/amd64, linux/arm64 |
| `huginn` | not checked | - |
| `hyperdx` | yes | linux/amd64, linux/arm64 |
| `imgcompress` | yes | linux/amd64, linux/arm64 |
| `imgproxy` | yes | linux/amd64, linux/arm64 |
| `immich` | yes | linux/amd64, linux/arm64 |
| `infisical` | yes | linux/amd64, linux/arm64 |
| `influxdb` | yes | linux/amd64, linux/arm64 |
| `inngest` | yes | linux/amd64, linux/arm64 |
| `invoice-ninja` | yes | linux/amd64, linux/arm64 |
| `isso` | not checked | - |
| `it-tools` | yes | linux/amd64, linux/arm64 |
| `jackett` | yes | linux/amd64, linux/arm64 |
| `jaeger` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/s390x |
| `jellyfin` | yes | linux/amd64, linux/arm64 |
| `jellyseerr` | yes | linux/amd64, linux/arm64 |
| `jellystat` | yes | linux/amd64, linux/arm64 |
| `jenkins` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `joomla` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/s390x |
| `joplin` | yes | linux/amd64, linux/arm64 |
| `joplin-server` | yes | linux/amd64, linux/arm64 |
| `jupyter-gpu` | amd64 only | linux/amd64 |
| `jupyter-notebook` | yes | linux/amd64, linux/arm64 |
| `jupyter-notebook-python` | yes | linux/amd64, linux/arm64 |
| `kanboard` | yes | linux/amd64, linux/arm, linux/arm64 |
| `karakeep` | yes | linux/amd64, linux/arm64 |
| `kavita` | yes | linux/amd64, linux/arm, linux/arm64 |
| `keycloak` | yes | linux/amd64, linux/arm64 |
| `keycloak-postgres` | yes | linux/amd64, linux/arm64 |
| `kimai` | yes | linux/amd64, linux/arm64 |
| `kokoro-fastapi` | yes | linux/amd64, linux/arm64 |
| `komga` | yes | linux/amd64, linux/arm, linux/arm64 |
| `kopia` | yes | linux/amd64, linux/arm, linux/arm64 |
| `kroki` | not checked | - |
| `kutt` | yes | linux/amd64, linux/arm64 |
| `label-studio` | yes | linux/amd64, linux/arm64 |
| `langflow` | yes | linux/amd64, linux/arm64 |
| `langfuse` | yes | linux/amd64, linux/arm64 |
| `languagetool` | yes | linux/amd64, linux/arm64 |
| `leantime` | yes | linux/amd64, linux/arm64 |
| `librechat` | yes | linux/amd64, linux/arm64 |
| `libreoffice` | yes | linux/amd64, linux/arm64 |
| `librespeed` | yes | linux/amd64, linux/arm, linux/arm64 |
| `libretranslate` | yes | linux/amd64, linux/arm64 |
| `lidarr` | yes | linux/amd64, linux/arm64 |
| `limesurvey` | yes | linux/amd64, linux/arm64 |
| `linkace` | yes | linux/amd64, linux/arm, linux/arm64 |
| `linkding` | yes | linux/amd64, linux/arm, linux/arm64 |
| `linkding-plus` | yes | linux/amd64, linux/arm, linux/arm64 |
| `linkwarden` | yes | linux/amd64, linux/arm64 |
| `listmonk` | yes | linux/amd64, linux/arm, linux/arm64 |
| `litellm` | yes | linux/amd64, linux/arm64 |
| `llama-cpp` | yes | linux/amd64, linux/arm64, linux/s390x |
| `lldap` | yes | linux/amd64, linux/arm, linux/arm64 |
| `lobe-chat` | yes | linux/amd64, linux/arm64 |
| `localai` | yes | linux/amd64, linux/arm64 |
| `logto` | yes | linux/amd64, linux/arm64 |
| `loki` | yes | linux/amd64, linux/arm, linux/arm64 |
| `lowcoder` | yes | linux/amd64, linux/arm64 |
| `lubelogger` | yes | linux/amd64, linux/arm64 |
| `lychee` | yes | linux/amd64, linux/arm64 |
| `mafl` | not checked | - |
| `mage-ai` | yes | linux/amd64, linux/arm64 |
| `mail-mox` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `maildev` | not checked | - |
| `mailhog` | amd64 only | linux/amd64 |
| `mailpit` | yes | linux/386, linux/amd64, linux/arm64 |
| `maintainerr` | yes | linux/amd64, linux/arm64 |
| `mariadb` | not checked | - |
| `marimo` | yes | linux/amd64, linux/arm64 |
| `matomo` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/s390x |
| `matrix-synapse` | yes | linux/amd64, linux/arm64 |
| `matrix-synapse-postgres` | yes | linux/amd64, linux/arm64 |
| `mattermost` | amd64 only | linux/amd64 |
| `mazanoke` | not checked | - |
| `mealie` | yes | linux/amd64, linux/arm64 |
| `mediawiki` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le |
| `meilisearch` | yes | linux/amd64, linux/arm64 |
| `memcached` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `memos` | yes | linux/amd64, linux/arm, linux/arm64 |
| `meshcentral` | yes | linux/amd64, linux/arm, linux/arm64 |
| `metabase` | amd64 only | linux/amd64 |
| `metamcp` | yes | linux/amd64, linux/arm64 |
| `metube` | yes | linux/amd64, linux/arm64 |
| `milvus` | yes | linux/amd64, linux/arm64 |
| `minecraft` | yes | linux/amd64, linux/arm64 |
| `minecraft-bedrock` | yes | linux/amd64, linux/arm64 |
| `miniflux` | yes | linux/amd64, linux/arm, linux/arm64 |
| `minio` | yes | linux/amd64, linux/arm64 |
| `mlflow` | yes | linux/amd64, linux/arm64 |
| `mongo-express` | yes | linux/amd64, linux/arm64 |
| `monica` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/s390x |
| `moodle` | yes | linux/amd64, linux/arm64 |
| `mosquitto` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `multi-scrobbler` | yes | linux/amd64, linux/arm64 |
| `mylar3` | yes | linux/amd64, linux/arm64 |
| `mysql` | not checked | - |
| `n8n` | yes | linux/amd64, linux/arm64 |
| `n8n-ai-starter` | yes | linux/amd64, linux/arm64 |
| `n8n-postgres` | yes | linux/amd64, linux/arm64 |
| `nats` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `navidrome` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `neo4j` | yes | linux/amd64, linux/arm64 |
| `netbox` | yes | linux/amd64, linux/arm64 |
| `netdata` | yes | linux/amd64, linux/arm, linux/arm64 |
| `new-api` | yes | linux/amd64, linux/arm64 |
| `nextcloud` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/mips64le, linux/ppc64le, linux/s390x |
| `nextjs-postgres-starter` | yes | linux/amd64, linux/arm, linux/arm64, linux/s390x |
| `nexus` | yes | linux/amd64, linux/arm64 |
| `nginx-multi-backend-starter` | yes | linux/amd64, linux/arm, linux/arm64, linux/s390x |
| `nginx-proxy-manager` | yes | linux/amd64, linux/arm, linux/arm64 |
| `nocobase` | yes | linux/amd64, linux/arm64 |
| `nocodb` | yes | linux/amd64, linux/arm, linux/arm64 |
| `node-exporter` | not checked | - |
| `node-postgres-starter` | yes | linux/amd64, linux/arm, linux/arm64, linux/s390x |
| `node-red` | yes | linux/amd64, linux/arm, linux/arm64 |
| `nodebb` | yes | linux/amd64, linux/arm64 |
| `ntfy` | yes | linux/amd64, linux/arm, linux/arm64 |
| `obsidian-livesync` | yes | linux/amd64, linux/arm64 |
| `odoo` | yes | linux/amd64, linux/arm64, linux/ppc64le |
| `ollama` | yes | linux/amd64, linux/arm64 |
| `ollama-deepseek` | yes | linux/amd64, linux/arm64 |
| `ollama-llama3` | yes | linux/amd64, linux/arm64 |
| `ollama-mistral` | yes | linux/amd64, linux/arm64 |
| `ollama-open-webui` | yes | linux/amd64, linux/arm64 |
| `ollama-phi` | yes | linux/amd64, linux/arm64 |
| `ollama-qwen` | yes | linux/amd64, linux/arm64 |
| `ombi` | yes | linux/amd64, linux/arm64 |
| `once-campfire` | yes | linux/amd64, linux/arm64 |
| `onedev` | yes | linux/amd64, linux/arm64 |
| `onetimesecret` | yes | linux/amd64, linux/arm64 |
| `open-notebook` | yes | linux/amd64, linux/arm64 |
| `open-webui` | yes | linux/amd64, linux/arm64 |
| `openclaw` | yes | linux/amd64, linux/arm64 |
| `opengist` | yes | linux/amd64, linux/arm64 |
| `openobserve` | yes | linux/amd64, linux/arm64 |
| `opensearch` | not checked | - |
| `openspeedtest` | not checked | - |
| `orangehrm` | amd64 only | linux/amd64 |
| `organizr` | yes | linux/amd64, linux/arm, linux/arm64 |
| `osticket` | not checked | - |
| `outline` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `overseerr` | yes | linux/amd64, linux/arm, linux/arm64 |
| `owncast` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `owncloud` | yes | linux/amd64, linux/arm64 |
| `pairdrop` | yes | linux/amd64, linux/arm64 |
| `palworld` | yes | linux/amd64, linux/arm64 |
| `paperless-ngx` | yes | linux/amd64, linux/arm64 |
| `passbolt` | yes | linux/amd64, linux/arm64 |
| `paymenter` | yes | linux/amd64, linux/arm64 |
| `penpot` | yes | linux/amd64, linux/arm64 |
| `pgadmin` | yes | linux/amd64, linux/arm64 |
| `pgbackweb` | yes | linux/amd64, linux/arm64 |
| `pgvector` | yes | linux/amd64, linux/arm64 |
| `pgweb` | yes | linux/amd64, linux/arm, linux/arm64 |
| `photoprism` | yes | linux/amd64, linux/arm64 |
| `photoview` | yes | linux/amd64, linux/arm64 |
| `phpmyadmin` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `pi-hole` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `pigallery2` | not checked | - |
| `pinchflat` | amd64 only | linux/amd64 |
| `pingvin-share` | yes | linux/amd64, linux/arm64 |
| `plane` | yes | linux/amd64, linux/arm64 |
| `planka` | yes | linux/amd64, linux/arm64 |
| `plantuml-server` | not checked | - |
| `plausible` | yes | linux/amd64, linux/arm64 |
| `plex` | yes | linux/amd64, linux/arm64 |
| `plunk` | yes | linux/amd64, linux/arm64 |
| `pocket-id` | yes | linux/amd64, linux/arm64 |
| `pocketbase` | yes | linux/amd64, linux/arm, linux/arm64 |
| `portainer` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, windows/amd64 |
| `postgres` | not checked | - |
| `postiz` | yes | linux/amd64, linux/arm64 |
| `prefect` | yes | linux/amd64, linux/arm64 |
| `privatebin` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `prometheus` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `prowlarr` | yes | linux/amd64, linux/arm64 |
| `proxyscotch` | yes | linux/amd64, linux/arm64 |
| `pushgateway` | not checked | - |
| `qbittorrent` | yes | linux/amd64, linux/arm64 |
| `qdrant` | yes | linux/amd64, linux/arm64 |
| `questdb` | not checked | - |
| `rabbitmq` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `radarr` | yes | linux/amd64, linux/arm64 |
| `radicale` | yes | linux/amd64, linux/arm, linux/arm64 |
| `rallly` | yes | linux/amd64, linux/arm64 |
| `reactive-resume` | yes | linux/amd64, linux/arm64 |
| `readeck` | yes | linux/amd64, linux/arm64 |
| `redash` | not checked | - |
| `redis-cache-starter` | yes | linux/amd64, linux/arm, linux/arm64, linux/s390x |
| `redis-commander` | not checked | - |
| `redis-insight` | yes | linux/amd64, linux/arm64 |
| `redisinsight` | yes | linux/amd64, linux/arm64 |
| `redlib` | yes | linux/amd64, linux/arm, linux/arm64 |
| `redmine` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `registry` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/s390x |
| `remark42` | not checked | - |
| `rocketchat` | yes | linux/amd64, linux/arm64 |
| `romm` | yes | linux/amd64, linux/arm64 |
| `roundcube` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `rss-bridge` | yes | linux/amd64, linux/arm, linux/arm64 |
| `rsshub` | yes | linux/amd64, linux/arm64 |
| `rustdesk-server` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `rustfs` | yes | linux/amd64, linux/arm64 |
| `ryot` | yes | linux/amd64, linux/arm64 |
| `sabnzbd` | yes | linux/amd64, linux/arm64 |
| `seafile` | yes | linux/amd64, linux/arm64 |
| `searxng` | yes | linux/amd64, linux/arm, linux/arm64 |
| `seaweedfs` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `seerr` | yes | linux/amd64, linux/arm64 |
| `semaphore` | yes | linux/amd64, linux/arm64 |
| `sftpgo` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le |
| `shaarli` | yes | linux/amd64, linux/arm, linux/arm64 |
| `shiori` | yes | linux/amd64, linux/arm, linux/arm64 |
| `shlink` | yes | linux/amd64, linux/arm64 |
| `silverbullet` | yes | linux/amd64, linux/arm, linux/arm64 |
| `siyuan` | yes | linux/amd64, linux/arm, linux/arm64 |
| `slash` | yes | linux/amd64, linux/arm64 |
| `smokeping` | yes | linux/amd64, linux/arm64 |
| `snapdrop` | not checked | - |
| `snipe-it` | yes | linux/amd64, linux/arm64 |
| `soketi` | yes | linux/amd64, linux/arm, linux/arm64 |
| `sonarqube` | yes | linux/amd64, linux/arm64 |
| `sonarr` | yes | linux/amd64, linux/arm64 |
| `sparkyfitness` | yes | linux/amd64, linux/arm64 |
| `speaches` | yes | linux/amd64, linux/arm64 |
| `speedtest-tracker` | yes | linux/amd64, linux/arm64 |
| `spliit` | yes | linux/amd64, linux/arm64 |
| `spoolman` | yes | linux/amd64, linux/arm, linux/arm64 |
| `stable-diffusion-webui` | amd64 only | linux/amd64 |
| `stalwart` | yes | linux/amd64, linux/arm, linux/arm64 |
| `static-site-api-starter` | yes | linux/amd64, linux/arm, linux/arm64, linux/s390x |
| `statusnook` | yes | linux/amd64, linux/arm64 |
| `stirling-pdf` | yes | linux/amd64, linux/arm64 |
| `strapi` | yes | linux/amd64, linux/arm64 |
| `superset` | yes | linux/amd64, linux/arm64 |
| `supertokens` | yes | linux/amd64, linux/arm64 |
| `sure` | yes | linux/amd64, linux/arm64 |
| `swagger-ui` | not checked | - |
| `syncthing` | yes | linux/amd64, linux/arm64 |
| `tandoor-recipes` | yes | linux/amd64, linux/arm64 |
| `tautulli` | yes | linux/amd64, linux/arm64 |
| `tdarr` | yes | linux/amd64, linux/arm64 |
| `teable` | yes | linux/amd64, linux/arm64 |
| `technitium-dns` | yes | linux/amd64, linux/arm, linux/arm64 |
| `temporal` | yes | linux/amd64, linux/arm64 |
| `termix` | yes | linux/amd64, linux/arm64 |
| `terraria` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le |
| `text-embeddings-inference` | amd64 only | linux/amd64 |
| `text-generation-inference` | amd64 only | linux/amd64 |
| `thelounge` | not checked | - |
| `tianji` | yes | linux/amd64, linux/arm64 |
| `timescaledb` | not checked | - |
| `tinyauth` | yes | linux/amd64, linux/arm64 |
| `tolgee` | yes | linux/amd64, linux/arm64 |
| `traccar` | yes | linux/amd64, linux/arm64 |
| `trailbase` | yes | linux/amd64, linux/arm64 |
| `transmission` | yes | linux/amd64, linux/arm64 |
| `trilium` | yes | linux/amd64, linux/arm, linux/arm64 |
| `triliumnext` | yes | linux/amd64, linux/arm, linux/arm64 |
| `twenty` | yes | linux/amd64, linux/arm64 |
| `typesense` | yes | linux/amd64, linux/arm64 |
| `umami` | yes | linux/amd64, linux/arm64 |
| `unleash-postgres` | yes | linux/amd64, linux/arm64 |
| `unstructured` | yes | linux/amd64, linux/arm64 |
| `uptime-kuma` | yes | linux/amd64, linux/arm, linux/arm64 |
| `uptime-kuma-mariadb` | yes | linux/amd64, linux/arm64 |
| `valkey` | yes | linux/amd64, linux/arm, linux/arm64, linux/ppc64le |
| `vault` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `vaultwarden` | yes | linux/amd64, linux/arm, linux/arm64 |
| `verdaccio` | yes | linux/amd64, linux/arm64 |
| `vert` | yes | linux/amd64, linux/arm64 |
| `victoriametrics` | yes | linux/386, linux/amd64, linux/arm, linux/arm64, linux/ppc64le |
| `vikunja` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `vllm` | yes | linux/amd64, linux/arm64 |
| `vvveb` | yes | linux/386, linux/amd64, linux/arm, linux/arm64 |
| `wakapi` | yes | linux/amd64, linux/arm, linux/arm64 |
| `wallabag` | yes | linux/amd64, linux/arm, linux/arm64 |
| `wallos` | yes | linux/amd64, linux/arm, linux/arm64 |
| `weaviate` | yes | linux/amd64, linux/arm64 |
| `web-check` | yes | linux/amd64, linux/arm64 |
| `weblate` | yes | linux/amd64, linux/arm64 |
| `wekan` | yes | linux/amd64, linux/arm64 |
| `whoogle` | yes | linux/amd64, linux/arm, linux/arm64 |
| `wikijs` | yes | linux/amd64, linux/arm64 |
| `windmill` | yes | linux/amd64, linux/arm64 |
| `wiremock` | not checked | - |
| `wizarr` | yes | linux/amd64, linux/arm64 |
| `woodpecker` | yes | freebsd/amd64, linux/amd64, linux/arm, linux/arm64, linux/ppc64le, linux/riscv64 |
| `wordpress` | yes | linux/amd64, linux/arm64 |
| `worker-redis-queue-starter` | yes | linux/amd64, linux/arm, linux/arm64, linux/s390x |
| `yamtrack` | yes | linux/amd64, linux/arm64 |
| `yourls` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/s390x |
| `zabbix` | yes | linux/amd64, linux/arm64, linux/ppc64le, linux/riscv64, linux/s390x |
| `zipkin` | not checked | - |
| `zipline` | yes | linux/amd64, linux/arm64 |
{/* END GENERATED ARM64 TABLE */}

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
