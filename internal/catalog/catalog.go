// Package catalog holds Levelrail's own curated set of one-click
// service templates (ADR 015). Every Template's name, slogan, and
// Compose body here is written fresh for this platform: none of it is
// copied or lightly-rewritten from any other project's own dataset.
// Image tags are real, versioned tags for each well-known open-source
// project; where a specific tag's exact string couldn't be verified
// against a live registry in this environment, that entry says so in
// its own comment.
package catalog

// Template is one deployable entry in the catalog. Compose is a full
// compose.yaml body, valid against internal/compose's supported
// subset (see catalog_test.go, which parses and validates every one).
type Template struct {
	ID               string
	Name             string
	Slogan           string
	Category         string
	DocumentationURL string
	Compose          string
}

// Templates is the full catalog, served by GET /api/v1/service-templates
// and GET /api/v1/service-templates/{id}.
var Templates = []Template{
	{
		ID:               "n8n",
		Name:             "n8n",
		Slogan:           "Build automations and connect your tools with a visual, node-based workflow editor.",
		Category:         "Automation",
		DocumentationURL: "https://docs.n8n.io",
		Compose: `services:
  n8n:
    image: n8nio/n8n:1.62.1
    ports: ["5678:5678"]
    environment:
      N8N_ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
      N8N_HOST: "0.0.0.0"
      N8N_PORT: "5678"
    volumes:
      - n8n_data:/home/node/.n8n
`,
	},
	{
		ID:               "uptime-kuma",
		Name:             "Uptime Kuma",
		Slogan:           "A self-hosted uptime monitor with a clean dashboard for HTTP, TCP, DNS, and ping checks.",
		Category:         "Monitoring",
		DocumentationURL: "https://github.com/louislam/uptime-kuma/wiki",
		Compose: `services:
  uptime-kuma:
    image: louislam/uptime-kuma:1.23.13
    ports: ["3001:3001"]
    volumes:
      - uptime_kuma_data:/app/data
`,
	},
	{
		ID:               "minio",
		Name:             "MinIO",
		Slogan:           "S3-compatible object storage you run yourself, with a built-in web console.",
		Category:         "Storage",
		DocumentationURL: "https://min.io/docs/minio/linux/index.html",
		// Real MinIO images require a "server /data" style command to
		// actually serve; the compose subset here doesn't parse
		// command:, so it's included for a human reader but has no
		// effect on the desired-state translation yet.
		Compose: `services:
  minio:
    image: minio/minio:RELEASE.2024-10-13T13-34-11Z
    command: ["server", "/data", "--console-address", ":9001"]
    ports: ["9000:9000", "9001:9001"]
    environment:
      MINIO_ROOT_USER: $SERVICE_USER_ROOT
      MINIO_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
    volumes:
      - minio_data:/data
`,
	},
	{
		ID:               "metabase",
		Name:             "Metabase",
		Slogan:           "Ask questions of your data and share dashboards, no SQL required.",
		Category:         "Analytics",
		DocumentationURL: "https://www.metabase.com/docs/latest/",
		Compose: `services:
  metabase:
    image: metabase/metabase:v0.50.8
    ports: ["3000:3000"]
    environment:
      MB_DB_FILE: /metabase-data/metabase.db
    volumes:
      - metabase_data:/metabase-data
`,
	},
	{
		ID:               "grafana",
		Name:             "Grafana",
		Slogan:           "Dashboards and exploration for metrics, logs, and traces from any data source.",
		Category:         "Monitoring",
		DocumentationURL: "https://grafana.com/docs/grafana/latest/",
		Compose: `services:
  grafana:
    image: grafana/grafana:11.2.0
    ports: ["3000:3000"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - grafana_data:/var/lib/grafana
`,
	},
	{
		ID:               "prometheus",
		Name:             "Prometheus",
		Slogan:           "A metrics time-series database and alerting engine built for pull-based scraping.",
		Category:         "Monitoring",
		DocumentationURL: "https://prometheus.io/docs/introduction/overview/",
		Compose: `services:
  prometheus:
    image: prom/prometheus:v2.54.1
    ports: ["9090:9090"]
    volumes:
      - prometheus_data:/prometheus
`,
	},
	{
		ID:               "portainer",
		Name:             "Portainer",
		Slogan:           "A web UI for managing containers, images, volumes, and networks.",
		Category:         "Infrastructure",
		DocumentationURL: "https://docs.portainer.io",
		// Portainer's usual setup bind-mounts the host Docker socket;
		// this platform's compose subset only supports named volumes
		// (no bind mounts), so socket access isn't wired up here yet.
		Compose: `services:
  portainer:
    image: portainer/portainer-ce:2.21.0
    ports: ["9443:9443"]
    volumes:
      - portainer_data:/data
`,
	},
	{
		ID:               "vaultwarden",
		Name:             "Vaultwarden",
		Slogan:           "A lightweight, self-hosted password manager server compatible with the Bitwarden clients.",
		Category:         "Security",
		DocumentationURL: "https://github.com/dani-garcia/vaultwarden/wiki",
		Compose: `services:
  vaultwarden:
    image: vaultwarden/server:1.32.1
    ports: ["8080:80"]
    environment:
      ADMIN_TOKEN: $SERVICE_HEX_64_ADMINTOKEN
    volumes:
      - vaultwarden_data:/data
`,
	},
	{
		ID:               "vikunja",
		Name:             "Vikunja",
		Slogan:           "An open-source task and project manager for teams that outgrew sticky notes.",
		Category:         "Productivity",
		DocumentationURL: "https://vikunja.io/docs/",
		Compose: `services:
  vikunja:
    image: vikunja/vikunja:0.24.1
    ports: ["3456:3456"]
    environment:
      VIKUNJA_SERVICE_JWTSECRET: $SERVICE_HEX_64_JWTSECRET
      VIKUNJA_DATABASE_TYPE: sqlite
    volumes:
      - vikunja_data:/app/vikunja/files
`,
	},
	{
		ID:               "outline",
		Name:             "Outline",
		Slogan:           "A fast, structured team wiki and knowledge base with real-time collaborative editing.",
		Category:         "Productivity",
		DocumentationURL: "https://docs.getoutline.com",
		Compose: `services:
  outline:
    image: outlinewiki/outline:0.79.0
    ports: ["3000:3000"]
    environment:
      SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      UTILS_SECRET: $SERVICE_HEX_64_UTILSSECRET
      DATABASE_URL: postgres://outline:$SERVICE_PASSWORD_DB@db:5432/outline
      REDIS_URL: redis://redis:6379
      URL: ${SERVICE_FQDN_OUTLINE:-http://localhost:3000}
      FORCE_HTTPS: "false"
    volumes:
      - outline_data:/var/lib/outline/data
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: outline
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: outline
    volumes:
      - outline_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - outline_redis_data:/data
`,
	},
	{
		ID:               "wordpress",
		Name:             "WordPress",
		Slogan:           "The world's most widely used content management system, self-hosted with its own database.",
		Category:         "Applications",
		DocumentationURL: "https://wordpress.org/documentation/",
		Compose: `services:
  wordpress:
    image: wordpress:6.6-apache
    ports: ["8080:80"]
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: wordpress
      WORDPRESS_DB_PASSWORD: $SERVICE_PASSWORD_DB
      WORDPRESS_DB_NAME: wordpress
    volumes:
      - wordpress_data:/var/www/html
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: wordpress
      MYSQL_USER: wordpress
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
    volumes:
      - wordpress_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "nextcloud",
		Name:             "Nextcloud",
		Slogan:           "Self-hosted file sync, sharing, and collaboration, a full private alternative to consumer cloud drives.",
		Category:         "Applications",
		DocumentationURL: "https://docs.nextcloud.com",
		Compose: `services:
  nextcloud:
    image: nextcloud:29.0.4-apache
    ports: ["8080:80"]
    environment:
      NEXTCLOUD_ADMIN_USER: $SERVICE_USER_ADMIN
      NEXTCLOUD_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - nextcloud_data:/var/www/html
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "umami",
		Name:             "Umami",
		Slogan:           "Simple, privacy-focused website analytics without tracking cookies or ad-tech.",
		Category:         "Analytics",
		DocumentationURL: "https://umami.is/docs",
		Compose: `services:
  umami:
    image: ghcr.io/umami-software/umami:postgresql-v2.15.0
    ports: ["3000:3000"]
    environment:
      DATABASE_TYPE: postgresql
      DATABASE_URL: postgresql://umami:$SERVICE_PASSWORD_DB@db:5432/umami
      APP_SECRET: $SERVICE_HEX_64_APPSECRET
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: umami
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: umami
    volumes:
      - umami_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "code-server",
		Name:             "code-server",
		Slogan:           "Run VS Code in the browser, on your own hardware, from any device with a tab open.",
		Category:         "Developer Tools",
		DocumentationURL: "https://coder.com/docs/code-server",
		Compose: `services:
  code-server:
    image: codercom/code-server:4.93.1
    ports: ["8080:8080"]
    environment:
      PASSWORD: $SERVICE_PASSWORD_CODE
    volumes:
      - code_server_data:/home/coder/project
`,
	},
	{
		ID:               "homepage",
		Name:             "Homepage",
		Slogan:           "A fast, static, highly customizable start page for all your self-hosted services.",
		Category:         "Dashboard",
		DocumentationURL: "https://gethomepage.dev/latest/",
		// Less certain than the other tags here that this exact patch
		// version is a real published tag for this fast-moving project;
		// the image repository and major line are correct.
		Compose: `services:
  homepage:
    image: ghcr.io/gethomepage/homepage:v0.10.4
    ports: ["3000:3000"]
    volumes:
      - homepage_config:/app/config
`,
	},
	{
		ID:               "audiobookshelf",
		Name:             "Audiobookshelf",
		Slogan:           "A self-hosted server for your audiobooks and podcasts, with sync across every device.",
		Category:         "Media",
		DocumentationURL: "https://www.audiobookshelf.org/docs",
		Compose: `services:
  audiobookshelf:
    image: ghcr.io/advplyr/audiobookshelf:2.34.0
    ports: ["80:80"]
    environment:
      TZ: "Etc/UTC"
    volumes:
      - audiobookshelf_config:/config
      - audiobookshelf_metadata:/metadata
      - audiobookshelf_audiobooks:/audiobooks
      - audiobookshelf_podcasts:/podcasts
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "calcom",
		Name:             "Cal.com",
		Slogan:           "Open-source scheduling infrastructure for booking meetings without the back-and-forth.",
		Category:         "Productivity",
		DocumentationURL: "https://cal.com/docs/self-hosting/installation",
		// calcom/cal.com's own registry doesn't publish a clean semver
		// tag list; this tag's exact string couldn't be verified against
		// a live registry in this environment.
		Compose: `services:
  calcom:
    image: calcom/cal.com:v5.0.9
    ports: ["3000:3000"]
    environment:
      NEXT_PUBLIC_WEBAPP_URL: ${SERVICE_FQDN_CALCOM:-http://localhost:3000}
      NEXTAUTH_URL: ${SERVICE_FQDN_CALCOM:-http://localhost:3000}
      NEXTAUTH_SECRET: $SERVICE_BASE64_NEXTAUTHSECRET
      CALENDSO_ENCRYPTION_KEY: $SERVICE_BASE64_ENCRYPTIONKEY
      DATABASE_URL: postgresql://calcom:$SERVICE_PASSWORD_DB@db:5432/calcom
      CALCOM_TELEMETRY_DISABLED: "1"
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: calcom
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: calcom
    volumes:
      - calcom_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "calibre-web",
		Name:             "Calibre-Web",
		Slogan:           "A clean web interface for browsing, reading, and downloading your existing Calibre ebook library.",
		Category:         "Media",
		DocumentationURL: "https://github.com/janeczku/calibre-web/wiki",
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  calibre-web:
    image: lscr.io/linuxserver/calibre-web:0.6.24
    ports: ["8083:8083"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - calibreweb_config:/config
      - calibreweb_books:/books
`,
	},
	{
		ID:               "convertx",
		Name:             "ConvertX",
		Slogan:           "A self-hosted file converter that handles well over a thousand image, document, and media formats.",
		Category:         "Developer Tools",
		DocumentationURL: "https://github.com/C4illin/ConvertX",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  convertx:
    image: ghcr.io/c4illin/convertx:0.19.0
    ports: ["3000:3000"]
    environment:
      JWT_SECRET: $SERVICE_PASSWORD_JWTSECRET
      ACCOUNT_REGISTRATION: "false"
      HTTP_ALLOWED: "true"
    volumes:
      - convertx_data:/app/data
`,
	},
	{
		ID:               "diun",
		Name:             "Diun",
		Slogan:           "Watches your running containers and notifies you the moment a new image tag is published.",
		Category:         "Monitoring",
		DocumentationURL: "https://crazymax.dev/diun/",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment. Diun's Docker provider normally watches
		// events via a bind-mounted /var/run/docker.sock, which this
		// platform's compose subset can't express yet (named volumes
		// only, no bind mounts), so that provider stays inactive until
		// bind-mount support lands.
		Compose: `services:
  diun:
    image: crazymax/diun:4.29.0
    environment:
      TZ: "Etc/UTC"
      LOG_LEVEL: "info"
      DIUN_WATCH_WORKERS: "20"
      DIUN_WATCH_SCHEDULE: "0 */6 * * *"
      DIUN_PROVIDERS_DOCKER: "true"
      DIUN_PROVIDERS_DOCKER_WATCHBYDEFAULT: "true"
    volumes:
      - diun_data:/data
`,
	},
	{
		ID:               "duplicati",
		Name:             "Duplicati",
		Slogan:           "Scheduled, encrypted backups of your files to local storage, network shares, or cloud storage.",
		Category:         "Storage",
		DocumentationURL: "https://duplicati.readthedocs.io",
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  duplicati:
    image: lscr.io/linuxserver/duplicati:2.1.1.0
    ports: ["8200:8200"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
      SETTINGS_ENCRYPTION_KEY: $SERVICE_PASSWORD_ENCRYPT
      DUPLICATI__WEBSERVICE_PASSWORD: $SERVICE_PASSWORD_WEB
    volumes:
      - duplicati_config:/config
      - duplicati_backups:/backups
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "formbricks",
		Name:             "Formbricks",
		Slogan:           "An open-source survey and experience-management platform you run on your own infrastructure.",
		Category:         "Productivity",
		DocumentationURL: "https://formbricks.com/docs/self-hosting/setup/docker",
		Compose: `services:
  formbricks:
    image: ghcr.io/formbricks/formbricks:4.5.0
    ports: ["3000:3000"]
    environment:
      WEBAPP_URL: ${SERVICE_FQDN_FORMBRICKS:-http://localhost:3000}
      NEXTAUTH_URL: ${SERVICE_FQDN_FORMBRICKS:-http://localhost:3000}
      NEXTAUTH_SECRET: $SERVICE_BASE64_NEXTAUTHSECRET
      ENCRYPTION_KEY: $SERVICE_BASE64_ENCRYPTIONKEY
      CRON_SECRET: $SERVICE_BASE64_CRONSECRET
      DATABASE_URL: postgresql://formbricks:$SERVICE_PASSWORD_DB@db:5432/formbricks
      REDIS_URL: redis://redis:6379
    volumes:
      - formbricks_uploads:/apps/web/uploads
  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: formbricks
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: formbricks
    volumes:
      - formbricks_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - formbricks_redis_data:/data
`,
	},
	{ //nolint:gosec // DB_CONNECTION_URI below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "infisical",
		Name:             "Infisical",
		Slogan:           "An open-source secrets manager to centralize API keys, database credentials, and app config.",
		Category:         "Security",
		DocumentationURL: "https://infisical.com/docs/self-hosting/overview",
		Compose: `services:
  infisical:
    image: infisical/infisical:v0.154.6
    ports: ["8080:8080"]
    environment:
      SITE_URL: ${SERVICE_FQDN_INFISICAL:-http://localhost:8080}
      ENCRYPTION_KEY: $SERVICE_PASSWORD_ENCRYPTIONKEY
      AUTH_SECRET: $SERVICE_REALBASE64_64_AUTHSECRET
      DB_CONNECTION_URI: postgres://infisical:$SERVICE_PASSWORD_DB@db:5432/infisical
      REDIS_URL: redis://redis:6379
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: infisical
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: infisical
    volumes:
      - infisical_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - infisical_redis_data:/data
`,
	},
	{
		ID:               "leantime",
		Name:             "Leantime",
		Slogan:           "A goals-focused project management tool built for people who aren't professional project managers.",
		Category:         "Productivity",
		DocumentationURL: "https://docs.leantime.io",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  leantime:
    image: leantime/leantime:3.5.6
    ports: ["8080:8080"]
    environment:
      LEAN_DB_HOST: db
      LEAN_DB_USER: leantime
      LEAN_DB_PASSWORD: $SERVICE_PASSWORD_DB
      LEAN_DB_DATABASE: leantime
      LEAN_SESSION_PASSWORD: $SERVICE_PASSWORD_64_SESSION
      LEAN_USE_REDIS: "true"
      LEAN_REDIS_HOST: redis
      LEAN_REDIS_PORT: "6379"
    volumes:
      - leantime_userfiles:/var/www/html/userfiles
      - leantime_public_userfiles:/var/www/html/public/userfiles
  db:
    image: mysql:8.4
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
      MYSQL_USER: leantime
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_DATABASE: leantime
    volumes:
      - leantime_db_data:/var/lib/mysql
  redis:
    image: redis:7-alpine
    volumes:
      - leantime_redis_data:/data
`,
	},
	{
		ID:               "librespeed",
		Name:             "LibreSpeed",
		Slogan:           "A lightweight, self-hosted internet speed test with no ads, tracking, or Flash required.",
		Category:         "Monitoring",
		DocumentationURL: "https://github.com/librespeed/speedtest",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  librespeed:
    image: ghcr.io/librespeed/speedtest:5.4.5
    ports: ["82:82"]
    environment:
      MODE: standalone
      TELEMETRY: "false"
      WEBPORT: "82"
`,
	},
	{
		ID:               "navidrome",
		Name:             "Navidrome",
		Slogan:           "Stream your own music collection from a Subsonic-compatible server to any device, anywhere.",
		Category:         "Media",
		DocumentationURL: "https://www.navidrome.org/docs/",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  navidrome:
    image: deluan/navidrome:0.54.1
    ports: ["4533:4533"]
    environment:
      ND_SCANSCHEDULE: "1h"
      ND_LOGLEVEL: "info"
    volumes:
      - navidrome_data:/data
      - navidrome_music:/music
`,
	},
	{
		ID:               "osticket",
		Name:             "osTicket",
		Slogan:           "A widely used open-source support ticket system for teams handling customer requests.",
		Category:         "Productivity",
		DocumentationURL: "https://docs.osticket.com/en/latest/",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  osticket:
    image: tiredofit/osticket:1.18.2
    ports: ["80:80"]
    environment:
      DB_HOST: db
      DB_NAME: osticket
      DB_USER: osticket
      DB_PASS: $SERVICE_PASSWORD_DB
      INSTALL_SECRET: $SERVICE_PASSWORD_INSTALLSECRET
      ADMIN_EMAIL: admin@example.com
      ADMIN_USER: $SERVICE_USER_ADMIN
      ADMIN_PASS: $SERVICE_PASSWORD_ADMIN
    volumes:
      - osticket_data:/www/osticket
  db:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_DBROOT
      MARIADB_DATABASE: osticket
      MARIADB_USER: osticket
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - osticket_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "overseerr",
		Name:             "Overseerr",
		Slogan:           "Lets your Plex users request new movies and TV shows straight from a shared web UI.",
		Category:         "Media",
		DocumentationURL: "https://docs.overseerr.dev",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  overseerr:
    image: sctx/overseerr:1.34.0
    ports: ["5055:5055"]
    environment:
      TZ: "Etc/UTC"
    volumes:
      - overseerr_config:/app/config
`,
	},
	{
		ID:               "passbolt",
		Name:             "Passbolt",
		Slogan:           "An open-source password manager built for teams, compatible with the usual browser extensions.",
		Category:         "Security",
		DocumentationURL: "https://www.passbolt.com/docs",
		// Only published under a rolling :latest-ce tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  passbolt:
    image: passbolt/passbolt:4.12.0-ce
    ports: ["80:80"]
    environment:
      APP_FULL_BASE_URL: ${SERVICE_FQDN_PASSBOLT:-http://localhost}
      PASSBOLT_SSL_FORCE: "false"
      DATASOURCES_DEFAULT_HOST: db
      DATASOURCES_DEFAULT_USERNAME: passbolt
      DATASOURCES_DEFAULT_PASSWORD: $SERVICE_PASSWORD_DB
      DATASOURCES_DEFAULT_DATABASE: passbolt
    volumes:
      - passbolt_gpg:/etc/passbolt/gpg
      - passbolt_jwt:/etc/passbolt/jwt
  db:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_DBROOT
      MARIADB_DATABASE: passbolt
      MARIADB_USER: passbolt
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - passbolt_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "penpot",
		Name:             "Penpot",
		Slogan:           "An open-source design and prototyping platform, a self-hosted alternative to Figma.",
		Category:         "Productivity",
		DocumentationURL: "https://help.penpot.app",
		Compose: `services:
  frontend:
    image: penpotapp/frontend:2.11.1
    ports: ["8080:8080"]
  backend:
    image: penpotapp/backend:2.11.1
    environment:
      PENPOT_FLAGS: "enable-login-with-password disable-email-verification"
      PENPOT_SECRET_KEY: $SERVICE_REALBASE64_64_SECRETKEY
      PENPOT_PUBLIC_URI: ${SERVICE_FQDN_FRONTEND:-http://localhost:8080}
      PENPOT_DATABASE_URI: postgresql://postgres/penpot
      PENPOT_DATABASE_USERNAME: $SERVICE_USER_POSTGRES
      PENPOT_DATABASE_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      PENPOT_REDIS_URI: redis://valkey/0
      PENPOT_OBJECTS_STORAGE_BACKEND: fs
      PENPOT_OBJECTS_STORAGE_FS_DIRECTORY: /opt/data/assets
    volumes:
      - penpot_assets:/opt/data/assets
  exporter:
    image: penpotapp/exporter:2.11.1
    environment:
      PENPOT_PUBLIC_URI: ${SERVICE_FQDN_FRONTEND:-http://localhost:8080}
      PENPOT_REDIS_URI: redis://valkey/0
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_INITDB_ARGS: "--data-checksums"
      POSTGRES_USER: $SERVICE_USER_POSTGRES
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: penpot
    volumes:
      - penpot_db_data:/var/lib/postgresql/data
  valkey:
    image: valkey/valkey:8.1-alpine
    volumes:
      - penpot_valkey_data:/data
`,
	},
	{
		ID:               "prowlarr",
		Name:             "Prowlarr",
		Slogan:           "An indexer manager that syncs your torrent and Usenet indexers across the whole Arr stack.",
		Category:         "Media",
		DocumentationURL: "https://wiki.servarr.com/prowlarr",
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  prowlarr:
    image: lscr.io/linuxserver/prowlarr:1.22.1
    ports: ["9696:9696"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - prowlarr_config:/config
`,
	},
	{
		ID:               "radarr",
		Name:             "Radarr",
		Slogan:           "Watches your favorite indexers for movies and automatically grabs, sorts, and renames them.",
		Category:         "Media",
		DocumentationURL: "https://wiki.servarr.com/radarr",
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  radarr:
    image: lscr.io/linuxserver/radarr:5.16.2
    ports: ["7878:7878"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - radarr_config:/config
`,
	},
	{
		ID:               "redisinsight",
		Name:             "RedisInsight",
		Slogan:           "A GUI for browsing keys, running commands, and profiling performance on any Redis instance.",
		Category:         "Database Tools",
		DocumentationURL: "https://redis.io/docs/latest/operate/redisinsight/",
		Compose: `services:
  redisinsight:
    image: redis/redisinsight:2.70
    ports: ["5540:5540"]
    environment:
      RI_APP_HOST: "0.0.0.0"
      RI_APP_PORT: "5540"
      RI_ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
    volumes:
      - redisinsight_data:/data
`,
	},
	{
		ID:               "soketi",
		Name:             "Soketi",
		Slogan:           "A simple, fast, Pusher-protocol-compatible WebSockets server for real-time app features.",
		Category:         "Developer Tools",
		DocumentationURL: "https://docs.soketi.app",
		Compose: `services:
  soketi:
    image: quay.io/soketi/soketi:1.6-16-debian
    ports: ["6001:6001"]
    environment:
      SOKETI_DEFAULT_APP_ID: $SERVICE_USER_SOKETI
      SOKETI_DEFAULT_APP_KEY: $SERVICE_REALBASE64_64_APPKEY
      SOKETI_DEFAULT_APP_SECRET: $SERVICE_REALBASE64_64_APPSECRET
      SOKETI_PUSHER_SCHEME: https
`,
	},
	{
		ID:               "sonarr",
		Name:             "Sonarr",
		Slogan:           "Watches your favorite indexers for new TV episodes and automatically grabs, sorts, and renames them.",
		Category:         "Media",
		DocumentationURL: "https://wiki.servarr.com/sonarr",
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  sonarr:
    image: lscr.io/linuxserver/sonarr:4.0.12
    ports: ["8989:8989"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - sonarr_config:/config
`,
	},
	{
		ID:               "tolgee",
		Name:             "Tolgee",
		Slogan:           "A localization management platform where developers and translators work in one shared UI.",
		Category:         "Developer Tools",
		DocumentationURL: "https://tolgee.io/platform",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  tolgee:
    image: tolgee/tolgee:3.92.0
    ports: ["8080:8080"]
    environment:
      TOLGEE_AUTHENTICATION_ENABLED: "true"
      TOLGEE_AUTHENTICATION_INITIAL_USERNAME: admin
      TOLGEE_AUTHENTICATION_INITIAL_PASSWORD: $SERVICE_PASSWORD_ADMIN
      TOLGEE_AUTHENTICATION_JWT_SECRET: $SERVICE_HEX_64_JWTSECRET
      TOLGEE_POSTGRES_AUTOSTART_ENABLED: "false"
      SPRING_DATASOURCE_URL: jdbc:postgresql://db:5432/tolgee
      SPRING_DATASOURCE_USERNAME: $SERVICE_USER_DB
      SPRING_DATASOURCE_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - tolgee_data:/data
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: tolgee
    volumes:
      - tolgee_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "weblate",
		Name:             "Weblate",
		Slogan:           "A continuous localization system for translating software with a web-based editor and review flow.",
		Category:         "Developer Tools",
		DocumentationURL: "https://docs.weblate.org",
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment. Weblate's redis normally needs
		// --requirepass via command: to actually enforce REDIS_PASSWORD;
		// command: has no effect in this platform's compose subset yet,
		// so the password below passes through unenforced for now.
		Compose: `services:
  weblate:
    image: weblate/weblate:5.9.2
    ports: ["8080:8080"]
    environment:
      WEBLATE_SITE_DOMAIN: ${SERVICE_FQDN_WEBLATE:-localhost}
      WEBLATE_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      POSTGRES_HOST: db
      POSTGRES_USER: weblate
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DATABASE: weblate
      REDIS_HOST: redis
      REDIS_PORT: "6379"
      REDIS_PASSWORD: $SERVICE_PASSWORD_REDIS
    volumes:
      - weblate_data:/app/data
      - weblate_cache:/app/cache
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: weblate
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: weblate
    volumes:
      - weblate_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    environment:
      REDIS_PASSWORD: $SERVICE_PASSWORD_REDIS
    volumes:
      - weblate_redis_data:/data
`,
	},
}
