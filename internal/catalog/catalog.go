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
}
