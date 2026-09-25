package catalog

var productivity1Templates = []Template{
	{
		ID:                     "vikunja",
		Name:                   "Vikunja",
		Slogan:                 "An open-source task and project manager for teams that outgrew sticky notes.",
		Category:               "Productivity",
		DocumentationURL:       "https://vikunja.io/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  vikunja:
    image: vikunja/vikunja:0.24.1
    ports: ["3456:3456"]
    environment:
      VIKUNJA_SERVICE_JWTSECRET: $SERVICE_HEX_64_JWTSECRET
      VIKUNJA_SERVICE_PUBLICURL: ${SERVICE_FQDN_VIKUNJA:-http://localhost:3456}
      # Vikunja's image runs as a fixed non-root uid with no chown step
      # of its own, so sqlite's default db path on a fresh named volume
      # is never writable; postgres avoids that entirely.
      VIKUNJA_DATABASE_TYPE: postgres
      VIKUNJA_DATABASE_HOST: db
      VIKUNJA_DATABASE_USER: vikunja
      VIKUNJA_DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
      VIKUNJA_DATABASE_DATABASE: vikunja
    volumes:
      # Persists across restarts, but attachment uploads still fail with
      # "permission denied" (verified live): same fixed-uid problem as
      # above, and this platform has no per-service user override yet.
      - vikunja_data:/app/vikunja/files
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3456/api/v1/info || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: vikunja
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: vikunja
    volumes:
      - vikunja_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "outline",
		Name:                   "Outline",
		Slogan:                 "A fast, structured team wiki and knowledge base with real-time collaborative editing.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.getoutline.com",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/_health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
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
		ID:                     "mealie",
		Name:                   "Mealie",
		Slogan:                 "A self-hosted recipe manager and meal planner with a clean web UI and API.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.mealie.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  mealie:
    image: ghcr.io/mealie-recipes/mealie:v3.17.0
    ports: ["9925:9000"]
    environment:
      BASE_URL: ${SERVICE_FQDN_MEALIE:-http://localhost:9925}
    volumes:
      - mealie_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9000/api/app/about"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "miniflux",
		Name:                   "Miniflux",
		Slogan:                 "A minimalist, fast RSS/Atom feed reader with no bloat and a keyboard-driven UI.",
		Category:               "Productivity",
		DocumentationURL:       "https://miniflux.app/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
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
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/healthcheck || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
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
		ID:                     "bookstack",
		Name:                   "BookStack",
		Slogan:                 "A simple, self-hosted platform for organizing documentation into books, chapters, and pages.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.bookstackapp.com/docs/",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/status"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 90s
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
		ID:                     "trilium",
		Name:                   "TriliumNext Notes",
		Slogan:                 "A hierarchical, self-hosted note-taking application built for large personal knowledge bases.",
		Category:               "Productivity",
		DocumentationURL:       "https://triliumnext.github.io/Docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  trilium:
    image: ghcr.io/triliumnext/trilium:stable
    ports: ["8080:8080"]
    volumes:
      - trilium_data:/home/node/trilium-data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/api/health-check"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "stirling-pdf",
		Name:                   "Stirling PDF",
		Slogan:                 "A self-hosted, all-in-one toolkit for merging, splitting, converting, and editing PDFs.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.stirlingpdf.com",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  stirling-pdf:
    image: stirlingtools/stirling-pdf:0.36.2
    ports: ["8080:8080"]
    volumes:
      - stirling_pdf_data:/usr/share/tessdata
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
`,
	},
	{
		ID:                     "paperless-ngx",
		Name:                   "Paperless-ngx",
		Slogan:                 "Scan, index, and archive your paper documents into a searchable digital library.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.paperless-ngx.com",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
  redis:
    image: redis:7.4
    volumes:
      - paperless_redis_data:/data
`,
	},
	{
		ID:                     "freshrss",
		Name:                   "FreshRSS",
		Slogan:                 "A lightweight, self-hosted RSS aggregator with multi-user support and a mobile-friendly API.",
		Category:               "Productivity",
		DocumentationURL:       "https://freshrss.github.io/FreshRSS/",
		RecommendedMemoryBytes: 268435456, // 256Mi
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
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
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
		ID:                     "joplin-server",
		Name:                   "Joplin Server",
		Slogan:                 "A self-hosted sync target for the Joplin note-taking app, replacing Dropbox or OneDrive sync.",
		Category:               "Productivity",
		DocumentationURL:       "https://joplinapp.org/help/api/server_config/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  joplin:
    image: joplin/server:3.7.2
    ports: ["22300:22300"]
    environment:
      APP_BASE_URL: ${SERVICE_FQDN_JOPLIN:-http://localhost:22300}
      DB_CLIENT: pg
      POSTGRES_HOST: db
      POSTGRES_DATABASE: joplin
      POSTGRES_USER: joplin
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:22300/api/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
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
		ID:                     "grocy",
		Name:                   "Grocy",
		Slogan:                 "A self-hosted ERP for your household: groceries, chores, and a shopping list that stays in sync.",
		Category:               "Productivity",
		DocumentationURL:       "https://grocy.info/en/docs",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  grocy:
    image: lscr.io/linuxserver/grocy:4.6.0
    ports: ["8080:80"]
    volumes:
      - grocy_data:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "kimai",
		Name:                   "Kimai",
		Slogan:                 "A self-hosted time tracking tool for freelancers and teams, with invoicing and reporting.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.kimai.org/documentation/",
		RecommendedMemoryBytes: 536870912, // 512Mi
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
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8001/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
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
		ID:                     "excalidraw",
		Name:                   "Excalidraw",
		Slogan:                 "A self-hosted virtual whiteboard for sketching diagrams that feel hand-drawn.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/excalidraw/excalidraw#docker",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		// Upstream publishes only latest and sha tags.
		Compose: `services:
  excalidraw:
    image: excalidraw/excalidraw:latest
    ports: ["8080:80"]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
}
