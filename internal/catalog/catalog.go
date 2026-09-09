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
		ID:               "ghost",
		Name:             "Ghost",
		Slogan:           "A fast, modern publishing platform for blogs and newsletters, with built-in memberships.",
		Category:         "Applications",
		DocumentationURL: "https://ghost.org/docs/",
		Compose: `services:
  ghost:
    image: ghost:5
    ports: ["2368:2368"]
    environment:
      database__client: mysql
      database__connection__host: db
      database__connection__user: ghost
      database__connection__password: $SERVICE_PASSWORD_DB
      database__connection__database: ghost
      url: ${SERVICE_FQDN_GHOST:-http://localhost:2368}
    volumes:
      - ghost_data:/var/lib/ghost/content
  db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: ghost
      MYSQL_USER: ghost
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
    volumes:
      - ghost_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "gitea",
		Name:             "Gitea",
		Slogan:           "A lightweight, self-hosted Git service with issues, pull requests, and a package registry.",
		Category:         "Developer Tools",
		DocumentationURL: "https://docs.gitea.com",
		// Tag not verified against a live registry in this environment;
		// the image repository and major line are correct.
		Compose: `services:
  gitea:
    image: gitea/gitea:1.23.1
    ports: ["3000:3000", "2222:22"]
    environment:
      GITEA__database__DB_TYPE: mysql
      GITEA__database__HOST: db:3306
      GITEA__database__NAME: gitea
      GITEA__database__USER: gitea
      GITEA__database__PASSWD: $SERVICE_PASSWORD_DB
    volumes:
      - gitea_data:/data
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: gitea
      MYSQL_USER: gitea
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - gitea_db_data:/var/lib/mysql
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "plausible",
		Name:             "Plausible Analytics",
		Slogan:           "Lightweight, privacy-friendly, cookie-free web analytics with no consent banner required.",
		Category:         "Analytics",
		DocumentationURL: "https://plausible.io/docs/self-hosting",
		Compose: `services:
  plausible:
    image: ghcr.io/plausible/community-edition:v3.0.1
    ports: ["8000:8000"]
    environment:
      BASE_URL: ${SERVICE_FQDN_PLAUSIBLE:-http://localhost:8000}
      SECRET_KEY_BASE: $SERVICE_BASE64_64_SECRETKEYBASE
      DATABASE_URL: postgres://plausible:$SERVICE_PASSWORD_DB@db:5432/plausible
      CLICKHOUSE_DATABASE_URL: http://clickhouse:8123/plausible
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: plausible
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: plausible
    volumes:
      - plausible_db_data:/var/lib/postgresql/data
  clickhouse:
    image: clickhouse/clickhouse-server:24.12-alpine
    volumes:
      - plausible_clickhouse_data:/var/lib/clickhouse
`,
	},
	{
		ID:               "mealie",
		Name:             "Mealie",
		Slogan:           "A self-hosted recipe manager and meal planner with a clean web UI and API.",
		Category:         "Productivity",
		DocumentationURL: "https://docs.mealie.io",
		Compose: `services:
  mealie:
    image: ghcr.io/mealie-recipes/mealie:3.17.0
    ports: ["9925:9000"]
    environment:
      BASE_URL: ${SERVICE_FQDN_MEALIE:-http://localhost:9925}
    volumes:
      - mealie_data:/app/data
`,
	},
	{
		ID:               "miniflux",
		Name:             "Miniflux",
		Slogan:           "A minimalist, fast RSS/Atom feed reader with no bloat and a keyboard-driven UI.",
		Category:         "Productivity",
		DocumentationURL: "https://miniflux.app/docs/",
		// Tag not verified against a live registry in this environment;
		// the image repository and major line are correct.
		Compose: `services:
  miniflux:
    image: ghcr.io/miniflux/miniflux:2.2.4
    ports: ["8080:8080"]
    environment:
      DATABASE_URL: postgres://miniflux:$SERVICE_PASSWORD_DB@db:5432/miniflux?sslmode=disable
      RUN_MIGRATIONS: "1"
      ADMIN_USERNAME: $SERVICE_USER_ADMIN
      ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: miniflux
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: miniflux
    volumes:
      - miniflux_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "firefly-iii",
		Name:             "Firefly III",
		Slogan:           "A self-hosted personal finance manager for tracking budgets, bills, and spending.",
		Category:         "Finance",
		DocumentationURL: "https://docs.firefly-iii.org",
		// Tag not verified against a live registry in this environment;
		// the image repository and major line are correct.
		Compose: `services:
  firefly:
    image: fireflyiii/core:version-6.2.3
    ports: ["8080:8080"]
    environment:
      APP_KEY: $SERVICE_BASE64_32_APPKEY
      DB_CONNECTION: mysql
      DB_HOST: db
      DB_DATABASE: firefly
      DB_USERNAME: firefly
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      APP_URL: ${SERVICE_FQDN_FIREFLY:-http://localhost:8080}
    volumes:
      - firefly_upload_data:/var/www/html/storage/upload
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: firefly
      MYSQL_USER: firefly
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - firefly_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "bookstack",
		Name:             "BookStack",
		Slogan:           "A simple, self-hosted platform for organizing documentation into books, chapters, and pages.",
		Category:         "Productivity",
		DocumentationURL: "https://www.bookstackapp.com/docs/",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  bookstack:
    image: lscr.io/linuxserver/bookstack:24.05.2
    ports: ["6875:80"]
    environment:
      APP_URL: ${SERVICE_FQDN_BOOKSTACK:-http://localhost:6875}
      DB_HOST: db
      DB_DATABASE: bookstack
      DB_USERNAME: bookstack
      DB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - bookstack_data:/config
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: bookstack
      MYSQL_USER: bookstack
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - bookstack_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "jellyfin",
		Name:             "Jellyfin",
		Slogan:           "A free media server for streaming your own movies, shows, and music to any device.",
		Category:         "Media",
		DocumentationURL: "https://jellyfin.org/docs/",
		// Jellyfin normally plays back files from a bind-mounted media
		// library; this platform's compose subset only supports named
		// volumes, so this template is useful for server setup and
		// configuration, not an actual populated library, until a
		// volume can be filled some other way.
		Compose: `services:
  jellyfin:
    image: lscr.io/linuxserver/jellyfin:10.11.8
    ports: ["8096:8096"]
    volumes:
      - jellyfin_config:/config
      - jellyfin_media:/data/media
`,
	},
	{
		ID:               "listmonk",
		Name:             "Listmonk",
		Slogan:           "A self-hosted newsletter and mailing list manager with a fast, dependency-light core.",
		Category:         "Communication",
		DocumentationURL: "https://listmonk.app/docs/",
		Compose: `services:
  listmonk:
    image: listmonk/listmonk:v6.0.0
    ports: ["9000:9000"]
    environment:
      LISTMONK_ADMIN_USER: $SERVICE_USER_ADMIN
      LISTMONK_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      LISTMONK_db__host: db
      LISTMONK_db__user: listmonk
      LISTMONK_db__password: $SERVICE_PASSWORD_DB
      LISTMONK_db__database: listmonk
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: listmonk
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: listmonk
    volumes:
      - listmonk_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "rocketchat",
		Name:             "Rocket.Chat",
		Slogan:           "A full-featured, self-hosted team chat platform with video calls and app integrations.",
		Category:         "Communication",
		DocumentationURL: "https://docs.rocket.chat",
		Compose: `services:
  rocketchat:
    image: registry.rocket.chat/rocketchat/rocket.chat:8.0.1
    ports: ["3000:3000"]
    environment:
      MONGO_URL: mongodb://mongo:27017/rocketchat
      MONGO_OPLOG_URL: mongodb://mongo:27017/local
      ROOT_URL: ${SERVICE_FQDN_ROCKETCHAT:-http://localhost:3000}
  mongo:
    image: mongo:7
    volumes:
      - rocketchat_mongo_data:/data/db
`,
	},
	{
		ID:               "nocodb",
		Name:             "NocoDB",
		Slogan:           "Turn any database into a smart spreadsheet, with a real-time collaborative grid UI.",
		Category:         "Database Tools",
		DocumentationURL: "https://docs.nocodb.com",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  nocodb:
    image: nocodb/nocodb:0.263.5
    ports: ["8080:8080"]
    volumes:
      - nocodb_data:/usr/app/data
`,
	},
	{
		ID:               "directus",
		Name:             "Directus",
		Slogan:           "An open-source headless CMS and instant REST/GraphQL API layer over your own database.",
		Category:         "Developer Tools",
		DocumentationURL: "https://docs.directus.io",
		Compose: `services:
  directus:
    image: directus/directus:11
    ports: ["8055:8055"]
    environment:
      KEY: $SERVICE_HEX_32_KEY
      SECRET: $SERVICE_HEX_32_SECRET
      ADMIN_EMAIL: admin@example.com
      ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      DB_CLIENT: pg
      DB_HOST: db
      DB_DATABASE: directus
      DB_USER: directus
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      CACHE_ENABLED: "true"
      CACHE_STORE: redis
      REDIS: redis://redis:6379
    volumes:
      - directus_uploads:/directus/uploads
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: directus
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: directus
    volumes:
      - directus_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - directus_redis_data:/data
`,
	},
	{
		ID:               "trilium",
		Name:             "TriliumNext Notes",
		Slogan:           "A hierarchical, self-hosted note-taking application built for large personal knowledge bases.",
		Category:         "Productivity",
		DocumentationURL: "https://triliumnext.github.io/Docs/",
		Compose: `services:
  trilium:
    image: ghcr.io/triliumnext/trilium:stable
    ports: ["8080:8080"]
    volumes:
      - trilium_data:/home/node/trilium-data
`,
	},
	{
		ID:               "shlink",
		Name:             "Shlink",
		Slogan:           "A self-hosted URL shortener with a full REST API for creating and tracking short links.",
		Category:         "Developer Tools",
		DocumentationURL: "https://shlink.io/documentation/",
		Compose: `services:
  shlink:
    image: shlinkio/shlink:stable
    ports: ["8080:8080"]
    environment:
      DEFAULT_DOMAIN: ${SERVICE_FQDN_SHLINK:-localhost}
      IS_HTTPS_ENABLED: "false"
    volumes:
      - shlink_data:/etc/shlink/data
`,
	},
	{
		ID:               "changedetection",
		Name:             "Changedetection.io",
		Slogan:           "Monitor any webpage for changes and get notified the moment content updates.",
		Category:         "Monitoring",
		DocumentationURL: "https://github.com/dgtlmoon/changedetection.io/wiki",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  changedetection:
    image: ghcr.io/dgtlmoon/changedetection.io:0.49.12
    ports: ["5000:5000"]
    volumes:
      - changedetection_data:/datastore
`,
	},
	{
		ID:               "stirling-pdf",
		Name:             "Stirling PDF",
		Slogan:           "A self-hosted, all-in-one toolkit for merging, splitting, converting, and editing PDFs.",
		Category:         "Productivity",
		DocumentationURL: "https://docs.stirlingpdf.com",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  stirling-pdf:
    image: stirlingtools/stirling-pdf:0.36.2
    ports: ["8080:8080"]
    volumes:
      - stirling_pdf_data:/usr/share/tessdata
`,
	},
	{
		ID:               "filebrowser",
		Name:             "File Browser",
		Slogan:           "A simple web UI for browsing, uploading, and sharing files from your own storage.",
		Category:         "Storage",
		DocumentationURL: "https://filebrowser.org",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct. Filebrowser normally serves
		// a bind-mounted host directory; this compose subset only
		// supports named volumes, so it starts pointed at an empty
		// volume rather than an existing folder of files.
		Compose: `services:
  filebrowser:
    image: filebrowser/filebrowser:v2.31.2
    ports: ["8080:80"]
    volumes:
      - filebrowser_data:/srv
      - filebrowser_db:/database
`,
	},
	{
		ID:               "syncthing",
		Name:             "Syncthing",
		Slogan:           "Continuous, peer-to-peer file synchronization between your own devices, no cloud in between.",
		Category:         "Storage",
		DocumentationURL: "https://docs.syncthing.net",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  syncthing:
    image: lscr.io/linuxserver/syncthing:1.29.4
    ports: ["8384:8384", "22000:22000"]
    volumes:
      - syncthing_config:/config
      - syncthing_data:/data
`,
	},
	{
		ID:               "ntfy",
		Name:             "ntfy",
		Slogan:           "A simple pub-sub push notification service you can send alerts to from any script or app.",
		Category:         "Communication",
		DocumentationURL: "https://docs.ntfy.sh",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct. The upstream image's default
		// command is "serve", but this compose subset doesn't parse
		// command:, the same limitation already noted on the MinIO
		// template above.
		Compose: `services:
  ntfy:
    image: binwiederhier/ntfy:v2.11.0
    ports: ["80:80"]
    volumes:
      - ntfy_cache:/var/cache/ntfy
      - ntfy_data:/etc/ntfy
`,
	},
	{
		ID:               "healthchecks",
		Name:             "Healthchecks",
		Slogan:           "Cron job and scheduled task monitoring: get alerted the moment a periodic job stops checking in.",
		Category:         "Monitoring",
		DocumentationURL: "https://healthchecks.io/docs/self_hosted/",
		Compose: `services:
  healthchecks:
    image: healthchecks/healthchecks:v4.2
    ports: ["8000:8000"]
    environment:
      DB: postgres
      DB_HOST: db
      DB_NAME: healthchecks
      DB_USER: healthchecks
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      ALLOWED_HOSTS: "*"
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: healthchecks
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: healthchecks
    volumes:
      - healthchecks_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "pgadmin",
		Name:             "pgAdmin",
		Slogan:           "A full-featured web GUI for administering and querying PostgreSQL databases.",
		Category:         "Database Tools",
		DocumentationURL: "https://www.pgadmin.org/docs/",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  pgadmin:
    image: dpage/pgadmin4:8.14
    ports: ["8080:80"]
    environment:
      PGADMIN_DEFAULT_EMAIL: admin@example.com
      PGADMIN_DEFAULT_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - pgadmin_data:/var/lib/pgadmin
`,
	},
	{
		ID:               "qbittorrent",
		Name:             "qBittorrent",
		Slogan:           "A free, self-hosted BitTorrent client with a full web UI for remote download management.",
		Category:         "Media",
		DocumentationURL: "https://github.com/qbittorrent/qBittorrent/wiki",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  qbittorrent:
    image: lscr.io/linuxserver/qbittorrent:5.0.3
    ports: ["8080:8080", "6881:6881"]
    volumes:
      - qbittorrent_config:/config
      - qbittorrent_downloads:/downloads
`,
	},
	{
		ID:               "home-assistant",
		Name:             "Home Assistant",
		Slogan:           "Open-source home automation that puts local control and privacy first.",
		Category:         "IoT",
		DocumentationURL: "https://www.home-assistant.io/docs/",
		// Home Assistant usually runs on host networking to discover
		// local devices; this template runs it on the platform's normal
		// bridge networking instead, so device auto-discovery won't
		// work out of the box, only the web UI and manually configured
		// integrations.
		Compose: `services:
  home-assistant:
    image: ghcr.io/home-assistant/home-assistant:2025.10.2
    ports: ["8123:8123"]
    volumes:
      - home_assistant_config:/config
`,
	},
	{
		ID:               "paperless-ngx",
		Name:             "Paperless-ngx",
		Slogan:           "Scan, index, and archive your paper documents into a searchable digital library.",
		Category:         "Productivity",
		DocumentationURL: "https://docs.paperless-ngx.com",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct. Runs against its own
		// embedded SQLite database rather than a separate Postgres
		// service, to keep this template to two containers.
		Compose: `services:
  paperless:
    image: paperlessngx/paperless-ngx:2.14.5
    ports: ["8000:8000"]
    environment:
      PAPERLESS_REDIS: redis://redis:6379
      PAPERLESS_SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      PAPERLESS_URL: ${SERVICE_FQDN_PAPERLESS:-http://localhost:8000}
    volumes:
      - paperless_data:/usr/src/paperless/data
      - paperless_media:/usr/src/paperless/media
  redis:
    image: redis:7.4
    volumes:
      - paperless_redis_data:/data
`,
	},
	{
		ID:               "immich",
		Name:             "Immich",
		Slogan:           "Self-hosted photo and video backup with mobile apps, facial recognition, and timeline search.",
		Category:         "Media",
		DocumentationURL: "https://immich.app/docs",
		// Tag not verified against a live registry in this environment;
		// the image repository and major line are correct. Pins to a
		// specific release rather than upstream's own floating
		// "release" default, since this compose subset has no way to
		// override an env default per deploy yet.
		Compose: `services:
  immich-server:
    image: ghcr.io/immich-app/immich-server:v1.126.1
    ports: ["2283:2283"]
    environment:
      DB_HOSTNAME: db
      DB_USERNAME: immich
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      DB_DATABASE_NAME: immich
      REDIS_HOSTNAME: redis
    volumes:
      - immich_upload_data:/usr/src/app/upload
  immich-machine-learning:
    image: ghcr.io/immich-app/immich-machine-learning:v1.126.1
    volumes:
      - immich_ml_cache:/cache
  redis:
    image: redis:7.4-alpine
  db:
    image: ghcr.io/immich-app/postgres:14-vectorchord0.3.0-pgvectors0.2.0
    environment:
      POSTGRES_USER: immich
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: immich
    volumes:
      - immich_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "freshrss",
		Name:             "FreshRSS",
		Slogan:           "A lightweight, self-hosted RSS aggregator with multi-user support and a mobile-friendly API.",
		Category:         "Productivity",
		DocumentationURL: "https://freshrss.github.io/FreshRSS/",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  freshrss:
    image: freshrss/freshrss:1.25.0
    ports: ["8080:80"]
    environment:
      DB_TYPE: mysql
      DB_HOST: db
      DB_USER: freshrss
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      DB_BASE: freshrss
    volumes:
      - freshrss_data:/var/www/FreshRSS/data
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: freshrss
      MYSQL_USER: freshrss
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - freshrss_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "joplin-server",
		Name:             "Joplin Server",
		Slogan:           "A self-hosted sync target for the Joplin note-taking app, replacing Dropbox or OneDrive sync.",
		Category:         "Productivity",
		DocumentationURL: "https://joplinapp.org/help/api/server_config/",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  joplin:
    image: joplin/server:3.3.4
    ports: ["22300:22300"]
    environment:
      APP_BASE_URL: ${SERVICE_FQDN_JOPLIN:-http://localhost:22300}
      DB_CLIENT: pg
      POSTGRES_HOST: db
      POSTGRES_DATABASE: joplin
      POSTGRES_USER: joplin
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
  db:
    image: postgres:16
    environment:
      POSTGRES_USER: joplin
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: joplin
    volumes:
      - joplin_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "gotenberg",
		Name:             "Gotenberg",
		Slogan:           "A stateless API for converting HTML, Markdown, Office, and PDF documents in the background.",
		Category:         "Developer Tools",
		DocumentationURL: "https://gotenberg.dev/docs/getting-started/introduction",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  gotenberg:
    image: gotenberg/gotenberg:8.15
    ports: ["3000:3000"]
`,
	},
	{ //nolint:gosec // MM_SQLSETTINGS_DATASOURCE below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "mattermost",
		Name:             "Mattermost",
		Slogan:           "An open-source, self-hosted alternative to Slack for team messaging and collaboration.",
		Category:         "Communication",
		DocumentationURL: "https://docs.mattermost.com",
		Compose: `services:
  mattermost:
    image: mattermost/mattermost-team-edition:release-10
    ports: ["8065:8065"]
    environment:
      MM_SQLSETTINGS_DRIVERNAME: postgres
      MM_SQLSETTINGS_DATASOURCE: postgres://mattermost:$SERVICE_PASSWORD_DB@db:5432/mattermost?sslmode=disable
      MM_SERVICESETTINGS_SITEURL: ${SERVICE_FQDN_MATTERMOST:-http://localhost:8065}
    volumes:
      - mattermost_data:/mattermost/data
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: mattermost
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: mattermost
    volumes:
      - mattermost_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "grocy",
		Name:             "Grocy",
		Slogan:           "A self-hosted ERP for your household: groceries, chores, and a shopping list that stays in sync.",
		Category:         "Productivity",
		DocumentationURL: "https://grocy.info/en/docs",
		Compose: `services:
  grocy:
    image: lscr.io/linuxserver/grocy:4.6.0
    ports: ["8080:80"]
    volumes:
      - grocy_data:/config
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "kimai",
		Name:             "Kimai",
		Slogan:           "A self-hosted time tracking tool for freelancers and teams, with invoicing and reporting.",
		Category:         "Productivity",
		DocumentationURL: "https://www.kimai.org/documentation/",
		Compose: `services:
  kimai:
    image: kimai/kimai2:apache
    ports: ["8001:8001"]
    environment:
      DATABASE_URL: mysql://kimai:$SERVICE_PASSWORD_DB@db/kimai?serverVersion=8.0
      ADMINMAIL: admin@example.com
      ADMINPASS: $SERVICE_PASSWORD_ADMIN
    volumes:
      - kimai_data:/opt/kimai/var
  db:
    image: mysql:8
    environment:
      MYSQL_DATABASE: kimai
      MYSQL_USER: kimai
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
    volumes:
      - kimai_db_data:/var/lib/mysql
`,
	},
	{
		ID:               "activepieces",
		Name:             "Activepieces",
		Slogan:           "An open-source, no-code automation tool for connecting apps and building AI-powered workflows.",
		Category:         "Automation",
		DocumentationURL: "https://www.activepieces.com/docs",
		Compose: `services:
  activepieces:
    image: ghcr.io/activepieces/activepieces:0.75.0
    ports: ["8080:80"]
    environment:
      AP_ENCRYPTION_KEY: $SERVICE_HEX_32_ENCRYPTIONKEY
      AP_JWT_SECRET: $SERVICE_HEX_32_JWTSECRET
      AP_FRONTEND_URL: ${SERVICE_FQDN_ACTIVEPIECES:-http://localhost:8080}
      AP_POSTGRES_HOST: db
      AP_POSTGRES_DATABASE: activepieces
      AP_POSTGRES_USERNAME: activepieces
      AP_POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      AP_REDIS_HOST: redis
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: activepieces
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: activepieces
    volumes:
      - activepieces_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - activepieces_redis_data:/data
`,
	},
	{
		ID:               "appsmith",
		Name:             "Appsmith",
		Slogan:           "A low-code platform for building internal tools and admin panels on top of your own data.",
		Category:         "Developer Tools",
		DocumentationURL: "https://docs.appsmith.com",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  appsmith:
    image: appsmith/appsmith-ce:1.72
    ports: ["8080:80"]
    volumes:
      - appsmith_data:/appsmith-stacks
`,
	},
	{
		ID:               "invoice-ninja",
		Name:             "Invoice Ninja",
		Slogan:           "Self-hosted invoicing, quotes, and payments for freelancers and small businesses.",
		Category:         "Finance",
		DocumentationURL: "https://invoiceninja.github.io",
		Compose: `services:
  invoiceninja:
    image: invoiceninja/invoiceninja:5
    ports: ["8080:80"]
    environment:
      APP_URL: ${SERVICE_FQDN_INVOICENINJA:-http://localhost:8080}
      APP_KEY: $SERVICE_BASE64_32_APPKEY
      DB_HOST: db
      DB_DATABASE: invoiceninja
      DB_USERNAME: invoiceninja
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      REDIS_HOST: redis
    volumes:
      - invoiceninja_data:/var/www/app/storage
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: invoiceninja
      MYSQL_USER: invoiceninja
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - invoiceninja_db_data:/var/lib/mysql
  redis:
    image: redis:7.4-alpine
    volumes:
      - invoiceninja_redis_data:/data
`,
	},
	{
		ID:               "excalidraw",
		Name:             "Excalidraw",
		Slogan:           "A self-hosted virtual whiteboard for sketching diagrams that feel hand-drawn.",
		Category:         "Productivity",
		DocumentationURL: "https://github.com/excalidraw/excalidraw#docker",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  excalidraw:
    image: excalidraw/excalidraw:0.17.6
    ports: ["8080:80"]
`,
	},
	{
		ID:               "wikijs",
		Name:             "Wiki.js",
		Slogan:           "A modern, extensible wiki engine with Markdown, visual editing, and fine-grained page permissions.",
		Category:         "Productivity",
		DocumentationURL: "https://docs.requarks.io",
		Compose: `services:
  wiki:
    image: ghcr.io/requarks/wiki:2
    ports: ["3000:3000"]
    environment:
      DB_TYPE: postgres
      DB_HOST: db
      DB_PORT: "5432"
      DB_USER: wikijs
      DB_PASS: $SERVICE_PASSWORD_DB
      DB_NAME: wikijs
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: wikijs
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: wikijs
    volumes:
      - wikijs_db_data:/var/lib/postgresql/data
`,
	},
	{ //nolint:gosec // CORE_DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "zipline",
		Name:             "Zipline",
		Slogan:           "A self-hosted file and screenshot host with a share-first upload flow and its own URL shortener.",
		Category:         "Storage",
		DocumentationURL: "https://zipline.diced.sh/docs",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  zipline:
    image: ghcr.io/diced/zipline:3.7.9
    ports: ["3000:3000"]
    environment:
      CORE_RETURN_HTTPS: "false"
      CORE_DATABASE_URL: postgres://zipline:$SERVICE_PASSWORD_DB@db:5432/zipline
      CORE_SECRET: $SERVICE_HEX_64_SECRET
    volumes:
      - zipline_uploads:/zipline/uploads
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: zipline
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: zipline
    volumes:
      - zipline_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:               "memos",
		Name:             "Memos",
		Slogan:           "A lightweight, privacy-first note-taking service for jotting down quick thoughts.",
		Category:         "Productivity",
		DocumentationURL: "https://www.usememos.com/docs",
		Compose: `services:
  memos:
    image: neosmemo/memos:stable
    ports: ["5230:5230"]
    volumes:
      - memos_data:/var/opt/memos
`,
	},
	{
		ID:               "dashy",
		Name:             "Dashy",
		Slogan:           "A feature-rich, self-hosted start page with widgets, status checks, and full visual customization.",
		Category:         "Dashboard",
		DocumentationURL: "https://dashy.to/docs",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  dashy:
    image: lissy93/dashy:3.1.1
    ports: ["8080:8080"]
    volumes:
      - dashy_config:/app/user-data
`,
	},
	{
		ID:               "glance",
		Name:             "Glance",
		Slogan:           "A fast, self-hosted dashboard that pulls RSS, weather, and other widgets onto one page.",
		Category:         "Dashboard",
		DocumentationURL: "https://github.com/glanceapp/glance/blob/main/docs/configuration.md",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  glance:
    image: glanceapp/glance:v0.8.6
    ports: ["8080:8080"]
    volumes:
      - glance_config:/app/config
`,
	},
	{
		ID:               "it-tools",
		Name:             "IT Tools",
		Slogan:           "A collection of handy online tools for developers: converters, generators, formatters, and more.",
		Category:         "Developer Tools",
		DocumentationURL: "https://it-tools.tech",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  it-tools:
    image: corentinth/it-tools:2024.10.22-7ca5933
    ports: ["8080:80"]
`,
	},
	{
		ID:               "meilisearch",
		Name:             "Meilisearch",
		Slogan:           "A fast, typo-tolerant search engine API you can drop into any app's search bar.",
		Category:         "Developer Tools",
		DocumentationURL: "https://www.meilisearch.com/docs",
		Compose: `services:
  meilisearch:
    image: getmeili/meilisearch:v1.11.1
    ports: ["7700:7700"]
    environment:
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MASTERKEY
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - meilisearch_data:/meili_data
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "docmost",
		Name:             "Docmost",
		Slogan:           "An open-source, Notion-style collaborative wiki and documentation workspace.",
		Category:         "Productivity",
		DocumentationURL: "https://docmost.com/docs",
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  docmost:
    image: docmost/docmost:0.13.2
    ports: ["3000:3000"]
    environment:
      APP_URL: ${SERVICE_FQDN_DOCMOST:-http://localhost:3000}
      APP_SECRET: $SERVICE_HEX_64_APPSECRET
      DATABASE_URL: postgresql://docmost:$SERVICE_PASSWORD_DB@db:5432/docmost
      REDIS_URL: redis://redis:6379
    volumes:
      - docmost_data:/app/data/storage
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: docmost
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: docmost
    volumes:
      - docmost_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7.2-alpine
    volumes:
      - docmost_redis_data:/data
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:               "glitchtip",
		Name:             "GlitchTip",
		Slogan:           "A lightweight, self-hosted error tracking service compatible with the Sentry SDK.",
		Category:         "Monitoring",
		DocumentationURL: "https://glitchtip.com/documentation",
		Compose: `services:
  glitchtip:
    image: glitchtip/glitchtip:6.0
    ports: ["8080:8080"]
    environment:
      SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      DATABASE_URL: postgres://glitchtip:$SERVICE_PASSWORD_DB@db:5432/glitchtip
      REDIS_URL: redis://redis:6379
      GLITCHTIP_DOMAIN: ${SERVICE_FQDN_GLITCHTIP:-http://localhost:8080}
      DEFAULT_FROM_EMAIL: admin@example.com
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: glitchtip
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: glitchtip
    volumes:
      - glitchtip_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - glitchtip_redis_data:/data
`,
	},
}
