package catalog

var devtools1Templates = []Template{
	{
		ID:                     "code-server",
		Name:                   "code-server",
		Slogan:                 "Run VS Code in the browser, on your own hardware, from any device with a tab open.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://coder.com/docs/code-server",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  code-server:
    image: codercom/code-server:4.93.1
    ports: ["8080:8080"]
    environment:
      PASSWORD: $SERVICE_PASSWORD_CODE
    volumes:
      - code_server_data:/home/coder/project
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "gitea",
		Name:                   "Gitea",
		Slogan:                 "A lightweight, self-hosted Git service with issues, pull requests, and a package registry.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.gitea.com",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/healthz || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
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
		ID:                     "plausible",
		Name:                   "Plausible Analytics",
		Slogan:                 "Lightweight, privacy-friendly, cookie-free web analytics with no consent banner required.",
		Category:               "Analytics",
		DocumentationURL:       "https://plausible.io/docs/self-hosting",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  plausible:
    image: ghcr.io/plausible/community-edition:v3.0.1
    ports: ["8000:8000"]
    environment:
      BASE_URL: ${SERVICE_FQDN_PLAUSIBLE:-http://localhost:8000}
      SECRET_KEY_BASE: $SERVICE_BASE64_64_SECRETKEYBASE
      DATABASE_URL: postgres://plausible:$SERVICE_PASSWORD_DB@db:5432/plausible
      CLICKHOUSE_DATABASE_URL: http://clickhouse:8123/plausible
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8000/api/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
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
		ID:                     "directus",
		Name:                   "Directus",
		Slogan:                 "An open-source headless CMS and instant REST/GraphQL API layer over your own database.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.directus.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8055/server/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
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
		ID:                     "shlink",
		Name:                   "Shlink",
		Slogan:                 "A self-hosted URL shortener with a full REST API for creating and tracking short links.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://shlink.io/documentation/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  shlink:
    image: shlinkio/shlink:stable
    ports: ["8080:8080"]
    environment:
      DEFAULT_DOMAIN: ${SERVICE_FQDN_SHLINK:-localhost}
      IS_HTTPS_ENABLED: "false"
    volumes:
      - shlink_data:/etc/shlink/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/rest/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "gotenberg",
		Name:                   "Gotenberg",
		Slogan:                 "A stateless API for converting HTML, Markdown, Office, and PDF documents in the background.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://gotenberg.dev/docs/getting-started/introduction",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  gotenberg:
    image: gotenberg/gotenberg:8.15
    ports: ["3000:3000"]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // MM_SQLSETTINGS_DATASOURCE below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "mattermost",
		Name:                   "Mattermost",
		Slogan:                 "An open-source, self-hosted alternative to Slack for team messaging and collaboration.",
		Category:               "Communication",
		DocumentationURL:       "https://docs.mattermost.com",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
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
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8065/api/v4/system/ping || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
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
		ID:                     "appsmith",
		Name:                   "Appsmith",
		Slogan:                 "A low-code platform for building internal tools and admin panels on top of your own data.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.appsmith.com",
		RecommendedMemoryBytes: 2147483648, // 2048Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  appsmith:
    image: appsmith/appsmith-ce:v2.4.2
    ports: ["8080:80"]
    volumes:
      - appsmith_data:/appsmith-stacks
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/api/v1/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 180s
`,
	},
	{
		ID:                     "it-tools",
		Name:                   "IT Tools",
		Slogan:                 "A collection of handy online tools for developers: converters, generators, formatters, and more.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://it-tools.tech",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  it-tools:
    image: corentinth/it-tools:2024.10.22-7ca5933
    ports: ["8080:80"]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "meilisearch",
		Name:                   "Meilisearch",
		Slogan:                 "A fast, typo-tolerant search engine API you can drop into any app's search bar.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://www.meilisearch.com/docs",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  meilisearch:
    image: getmeili/meilisearch:v1.11.1
    ports: ["7700:7700"]
    environment:
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MASTERKEY
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - meilisearch_data:/meili_data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:7700/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "docmost",
		Name:                   "Docmost",
		Slogan:                 "An open-source, Notion-style collaborative wiki and documentation workspace.",
		Category:               "Productivity",
		DocumentationURL:       "https://docmost.com/docs",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  docmost:
    image: docmost/docmost:0.96.0
    ports: ["3000:3000"]
    environment:
      APP_URL: ${SERVICE_FQDN_DOCMOST:-http://localhost:3000}
      APP_SECRET: $SERVICE_HEX_64_APPSECRET
      DATABASE_URL: postgresql://docmost:$SERVICE_PASSWORD_DB@db:5432/docmost
      REDIS_URL: redis://redis:6379
    volumes:
      - docmost_data:/app/data/storage
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
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
		ID:                     "glitchtip",
		Name:                   "GlitchTip",
		Slogan:                 "A lightweight, self-hosted error tracking service compatible with the Sentry SDK.",
		Category:               "Monitoring",
		DocumentationURL:       "https://glitchtip.com/documentation",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/_health/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
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
	{
		ID:                     "convertx",
		Name:                   "ConvertX",
		Slogan:                 "A self-hosted file converter that handles well over a thousand image, document, and media formats.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/C4illin/ConvertX",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		// Upstream publishes no release tags on ghcr.
		Compose: `services:
  convertx:
    image: ghcr.io/c4illin/convertx:latest
    ports: ["3000:3000"]
    environment:
      JWT_SECRET: $SERVICE_PASSWORD_JWTSECRET
      ACCOUNT_REGISTRATION: "false"
      HTTP_ALLOWED: "true"
    volumes:
      - convertx_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "soketi",
		Name:                   "Soketi",
		Slogan:                 "A simple, fast, Pusher-protocol-compatible WebSockets server for real-time app features.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.soketi.app",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  soketi:
    image: quay.io/soketi/soketi:1.6-16-debian
    ports: ["6001:6001"]
    environment:
      SOKETI_DEFAULT_APP_ID: $SERVICE_USER_SOKETI
      SOKETI_DEFAULT_APP_KEY: $SERVICE_REALBASE64_64_APPKEY
      SOKETI_DEFAULT_APP_SECRET: $SERVICE_REALBASE64_64_APPSECRET
      SOKETI_PUSHER_SCHEME: https
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:6001/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
}
