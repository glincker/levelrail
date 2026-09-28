package catalog

var applicationsTemplates = []Template{
	{
		ID:                     "wordpress",
		Name:                   "WordPress",
		Slogan:                 "The world's most widely used content management system, self-hosted with its own database.",
		Category:               "Applications",
		DocumentationURL:       "https://wordpress.org/documentation/",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
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
		ID:                     "nextcloud",
		Name:                   "Nextcloud",
		Slogan:                 "Self-hosted file sync, sharing, and collaboration, a full private alternative to consumer cloud drives.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.nextcloud.com",
		RecommendedMemoryBytes: 1610612736, // 1536Mi
		Compose: `services:
  nextcloud:
    image: nextcloud:29.0.4-apache
    ports: ["8080:80"]
    environment:
      NEXTCLOUD_ADMIN_USER: $SERVICE_USER_ADMIN
      NEXTCLOUD_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - nextcloud_data:/var/www/html
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/status.php || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "umami",
		Name:                   "Umami",
		Slogan:                 "Simple, privacy-focused website analytics without tracking cookies or ad-tech.",
		Category:               "Analytics",
		DocumentationURL:       "https://umami.is/docs",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  umami:
    image: ghcr.io/umami-software/umami:postgresql-v2.15.0
    ports: ["3000:3000"]
    environment:
      DATABASE_TYPE: postgresql
      DATABASE_URL: postgresql://umami:$SERVICE_PASSWORD_DB@db:5432/umami
      APP_SECRET: $SERVICE_HEX_64_APPSECRET
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/heartbeat || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
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
		ID:                     "ghost",
		Name:                   "Ghost",
		Slogan:                 "A fast, modern publishing platform for blogs and newsletters, with built-in memberships.",
		Category:               "Applications",
		DocumentationURL:       "https://ghost.org/docs/",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:2368/ghost/api/admin/site/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
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
		ID:                     "searxng",
		Name:                   "SearXNG",
		Slogan:                 "A privacy-respecting metasearch engine that aggregates results from dozens of search services.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.searxng.org",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Real SearXNG setups mount a custom settings.yml; this platform's
		// compose subset has no bind-mount support, so it boots on the
		// image's own default settings instead. Only published under a
		// rolling :latest tag upstream; this pinned version couldn't be
		// verified against a live registry in this environment.
		Compose: `services:
  searxng:
    image: searxng/searxng:2026.9.23-3cd69d30e
    ports: ["8080:8080"]
    environment:
      SEARXNG_SECRET: $SERVICE_HEX_64_SECRET
      SEARXNG_REDIS_URL: redis://redis:6379/0
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  redis:
    image: redis:7-alpine
    volumes:
      - searxng_redis_data:/data
`,
	},
	{
		ID:                     "redlib",
		Name:                   "Redlib",
		Slogan:                 "A private, lightweight front-end for browsing Reddit without tracking or ads.",
		Category:               "Applications",
		DocumentationURL:       "https://github.com/redlib-org/redlib",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Upstream only publishes a rolling "latest" tag plus per-commit
		// "sha-*" builds, no semver releases; pinned to a specific
		// commit-built image instead.
		Compose: `services:
  redlib:
    image: quay.io/redlib/redlib:sha-a4d36e9
    ports: ["8080:8080"]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/settings"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
}
