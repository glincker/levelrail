package catalog

// catalogBatch3Templates is a fourth wave of the Coolify-catalog import
// (ADR 015), picked from genuine gaps against the catalog shipped so
// far. Dropped: garage/soju/opnform (bind-mounted config), plunk/
// usesend (AWS SES creds with no default), traccar (bind-mounted JDBC
// config), mixpost (boots fine, but takes over 5 minutes to migrate
// and start), and slugs already shipped under a different ID
// (docuseal, joomla, drupal, label-studio, Postgres Gitea/Forgejo/
// Keycloak, pocket-id, vikunja, outline, paperless, freshrss, n8n).
var catalogBatch3Templates = []Template{
	{
		ID:                     "sure",
		Name:                   "Sure",
		Slogan:                 "A privacy-first personal finance app for tracking net worth, budgets, and investments across every account.",
		Category:               "Finance",
		DocumentationURL:       "https://github.com/we-promise/sure",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  web:
    image: ghcr.io/we-promise/sure:0.6.7
    ports: ["3000:3000"]
    environment:
      APP_DOMAIN: ${SERVICE_FQDN_SURE:-http://localhost:3000}
      SECRET_KEY_BASE: $SERVICE_BASE64_64_SURE
      POSTGRES_USER: sure
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRESQL
      POSTGRES_DB: sure
      REDIS_URL: redis://valkey:6379
      DB_HOST: postgresql
      DB_PORT: "5432"
      SELF_HOSTED: "true"
      RAILS_FORCE_SSL: "false"
      RAILS_ASSUME_SSL: "false"
      ONBOARDING_STATE: open
    volumes:
      - sure_app_storage:/rails/storage
    depends_on: [postgresql, valkey]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:3000"]
      interval: 15s
      timeout: 20s
      retries: 5
      start_period: 20s
  worker:
    image: ghcr.io/we-promise/sure:0.6.7
    command: ["bundle", "exec", "sidekiq"]
    environment:
      SECRET_KEY_BASE: $SERVICE_BASE64_64_SURE
      POSTGRES_USER: sure
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRESQL
      POSTGRES_DB: sure
      REDIS_URL: redis://valkey:6379
      DB_HOST: postgresql
      DB_PORT: "5432"
      SELF_HOSTED: "true"
    volumes:
      - sure_app_storage:/rails/storage
    depends_on: [postgresql, valkey]
  postgresql:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: sure
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRESQL
      POSTGRES_DB: sure
    volumes:
      - sure_postgresql_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U sure -d sure"]
      interval: 5s
      timeout: 20s
      retries: 10
  valkey:
    image: valkey/valkey:8-alpine
    command: ["valkey-server", "--appendonly", "yes"]
    volumes:
      - sure_valkey_data:/data
    healthcheck:
      test: ["CMD-SHELL", "valkey-cli ping | grep -q PONG"]
      interval: 5s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "vert",
		Name:                   "Vert",
		Slogan:                 "A fast file converter for images, video, audio, and documents, processed entirely without a third-party upload.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/vert-sh/vert",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Only published under a rolling :latest tag upstream.
		Compose: `services:
  vert:
    image: ghcr.io/vert-sh/vert:latest
    ports: ["80:80"]
    environment:
      PUB_VERT_URL: ${SERVICE_FQDN_VERT:-http://localhost:80}
      PUB_HOSTNAME: ${SERVICE_FQDN_VERT:-http://localhost:80}
      PUB_PORT: "80"
      PUB_ENV: production
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:80/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 10s
`,
	},
	{
		ID:                     "organizr",
		Name:                   "Organizr",
		Slogan:                 "A unified homepage and tabbed dashboard for linking every self-hosted app behind one interface.",
		Category:               "Dashboard",
		DocumentationURL:       "https://docs.organizr.app/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  organizr:
    image: organizr/organizr:latest
    ports: ["80:80"]
    environment:
      branch: v2-master
    volumes:
      - organizr_data:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -sf http://127.0.0.1:80 || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // PBW_ENCRYPTION_KEY/POSTGRES_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "pgbackweb",
		Name:                   "PG Back Web",
		Slogan:                 "A web UI for scheduling, encrypting, and restoring Postgres backups, with its own Postgres instance to protect.",
		Category:               "Database Tools",
		DocumentationURL:       "https://pgbackweb.com/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  pgbackweb:
    image: eduardolat/pgbackweb:latest
    ports: ["8085:8085"]
    environment:
      PBW_ENCRYPTION_KEY: $SERVICE_PASSWORD_64_PGBACKWEB
      PBW_POSTGRES_CONN_STRING: postgresql://pgbackweb:$SERVICE_PASSWORD_POSTGRES@postgres:5432/pgbackweb-db?sslmode=disable
      TZ: UTC
    volumes:
      - pgbackweb_backups:/backups
    depends_on: [postgres]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8085/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 15s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: pgbackweb
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: pgbackweb-db
    volumes:
      - pgbackweb_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U pgbackweb -d pgbackweb-db"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "ollama-open-webui",
		Name:                   "Ollama + Open WebUI",
		Slogan:                 "A local LLM runtime paired with a ChatGPT-style web interface, for running open models on your own hardware.",
		Category:               "AI",
		DocumentationURL:       "https://docs.openwebui.com/",
		RecommendedMemoryBytes: 2147483648, // 2048Mi
		Compose: `services:
  ollama-api:
    image: ollama/ollama:latest
    volumes:
      - ollama_data:/root/.ollama
    healthcheck:
      test: ["CMD", "ollama", "list"]
      interval: 5s
      timeout: 30s
      retries: 10
  open-webui:
    image: ghcr.io/open-webui/open-webui:main
    ports: ["8080:8080"]
    environment:
      OLLAMA_BASE_URL: http://ollama-api:11434
    volumes:
      - open_webui_data:/app/backend/data
    depends_on: [ollama-api]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:8080"]
      interval: 5s
      timeout: 30s
      retries: 10
`,
	},
	{ //nolint:gosec // SECRET_KEY_BASE below is a compose magic-var token, not a real credential
		ID:                     "campfire",
		Name:                   "Campfire",
		Slogan:                 "Basecamp's open-source group chat app: rooms, direct messages, and file sharing, self-hosted as one container.",
		Category:               "Communication",
		DocumentationURL:       "https://github.com/basecamp/once-campfire",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling "main" tag upstream, no
		// versioned release yet.
		Compose: `services:
  campfire:
    image: ghcr.io/basecamp/once-campfire:main
    ports: ["80:80"]
    environment:
      SECRET_KEY_BASE: $SERVICE_BASE64_64_CAMPFIRE
      DISABLE_SSL: "true"
      SSL_DOMAIN: "false"
      SKIP_TELEMETRY: "true"
    volumes:
      - campfire_storage:/rails/storage
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1/up"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
`,
	},
	{ //nolint:gosec // POSTGRES_PASSWORD below is a compose magic-var token, not a real credential
		ID:                     "supertokens",
		Name:                   "SuperTokens",
		Slogan:                 "A self-hosted authentication backend with session management, social login, and passwordless, backed by Postgres.",
		Category:               "Security",
		DocumentationURL:       "https://supertokens.com/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// No curl/wget in this image; GET /hello via a raw /dev/tcp
		// connection is the documented check, so it has no HTTP-probe
		// translation here (see catalog_test.go's tcpOnlyTemplates).
		Compose: `services:
  supertokens:
    image: registry.supertokens.io/supertokens/supertokens-postgresql:latest
    ports: ["3567:3567"]
    environment:
      API_KEYS: ""
      POSTGRESQL_CONNECTION_URI: postgresql://supertokens:$SERVICE_PASSWORD_POSTGRESQL@postgres:5432/supertokens
    depends_on: [postgres]
    healthcheck:
      test: ["CMD-SHELL", "bash -c 'exec 3<>/dev/tcp/127.0.0.1/3567 && echo -e \"GET /hello HTTP/1.1\\r\\nhost: 127.0.0.1:3567\\r\\nConnection: close\\r\\n\\r\\n\" >&3 && cat <&3 | grep \"Hello\"'"]
      interval: 10s
      timeout: 5s
      retries: 5
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: supertokens
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRESQL
      POSTGRES_DB: supertokens
    volumes:
      - supertokens_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U supertokens -d supertokens"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "goatcounter",
		Name:                   "GoatCounter",
		Slogan:                 "A privacy-friendly, open-source web analytics platform that never tracks individual visitors.",
		Category:               "Analytics",
		DocumentationURL:       "https://www.goatcounter.com/help",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  goatcounter:
    image: arp242/goatcounter:2.7
    ports: ["8080:8080"]
    volumes:
      - goatcounter_data:/home/goatcounter/goatcounter-data
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/status >/dev/null 2>&1 || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "slash",
		Name:                   "Slash",
		Slogan:                 "A self-hosted, Markdown-friendly bookmark manager and link shortener for organizing team knowledge.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/yourselfhosted/slash",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Only published under a rolling :latest tag upstream.
		Compose: `services:
  slash:
    image: yourselfhosted/slash:latest
    ports: ["5231:5231"]
    volumes:
      - slash_data:/var/opt/slash
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://127.0.0.1:5231"]
      interval: 10s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "web-check",
		Name:                   "Web Check",
		Slogan:                 "An all-in-one OSINT tool for auditing a website's DNS, SSL, headers, performance, and security posture.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/Lissy93/web-check",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream.
		Compose: `services:
  web-check:
    image: lissy93/web-check:latest
    ports: ["3000:3000"]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:3000/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "cryptgeon",
		Name:                   "Cryptgeon",
		Slogan:                 "A self-destructing secret sharing tool with end-to-end encryption, view limits, and expiration timers.",
		Category:               "Security",
		DocumentationURL:       "https://github.com/cupcakearmy/cryptgeon",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  app:
    image: cupcakearmy/cryptgeon:latest
    ports: ["8000:8000"]
    environment:
      SIZE_LIMIT: "4 MiB"
      MAX_VIEWS: "100"
      MAX_EXPIRATION: "360"
      ALLOW_ADVANCED: "true"
      ALLOW_FILES: "true"
    depends_on: [redis]
    healthcheck:
      test: ["CMD", "curl", "--fail", "http://127.0.0.1:8000/api/live/"]
      interval: 1m
      timeout: 5s
      retries: 3
      start_period: 10s
  redis:
    image: redis:7-alpine
    command: ["redis-server", "--maxmemory", "200mb", "--maxmemory-policy", "allkeys-lru"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5
`,
	},
	{ //nolint:gosec // POSTGRES_PASSWORD below is a compose magic-var token, not a real credential
		ID:                     "nodebb",
		Name:                   "NodeBB",
		Slogan:                 "A modern forum platform with real-time discussions, SSO, and a plugin ecosystem, backed by Postgres.",
		Category:               "Communication",
		DocumentationURL:       "https://docs.nodebb.org/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// No curl/wget in this image; a raw /dev/tcp connect is the
		// documented check, so it has no HTTP-probe translation here
		// (see catalog_test.go's tcpOnlyTemplates). The setup.json this
		// writes at container start is NodeBB's own documented
		// non-interactive config path; $POSTGRES_PASSWORD is this
		// service's own real env var (set below), expanded by the
		// shell at container start, not a magic-var token.
		Compose: `services:
  nodebb:
    image: ghcr.io/nodebb/nodebb:latest
    ports: ["4567:4567"]
    environment:
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
    volumes:
      - nodebb_build:/usr/src/app/build
      - nodebb_uploads:/usr/src/app/public/uploads
      - nodebb_config:/opt/config
    command:
      - /bin/sh
      - -c
      - |
        cat > /usr/src/app/setup.json <<EOF
        {
            "defaults": {
                "postgres": {
                    "host": "postgres",
                    "port": 5432,
                    "database": "nodebb",
                    "username": "nodebb",
                    "password": "$POSTGRES_PASSWORD"
                }
            }
        }
        EOF
        exec tini -- entrypoint.sh
    depends_on: [postgres]
    healthcheck:
      test: ["CMD-SHELL", "bash -c ':> /dev/tcp/127.0.0.1/4567' || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 30s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: nodebb
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: nodebb
    volumes:
      - nodebb_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nodebb -d nodebb"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // SECRET_KEY/MYSQL_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "bugsink",
		Name:                   "Bugsink",
		Slogan:                 "A self-hosted, lightweight error tracking server with a Sentry-compatible SDK ingestion API.",
		Category:               "Monitoring",
		DocumentationURL:       "https://www.bugsink.com/docs/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream.
		Compose: `services:
  web:
    image: bugsink/bugsink:latest
    ports: ["8000:8000"]
    environment:
      SECRET_KEY: $SERVICE_PASSWORD_64_SECRETKEY
      CREATE_SUPERUSER: admin:$SERVICE_PASSWORD_BUGSINK
      BASE_URL: ${SERVICE_FQDN_BUGSINK:-http://localhost:8000}
      DATABASE_URL: mysql://bugsink:$SERVICE_PASSWORD_BUGSINK@mysql:3306/bugsink
      BEHIND_HTTPS_PROXY: "True"
    depends_on: [mysql]
    healthcheck:
      test: ["CMD-SHELL", "python -c 'import requests; requests.get(\"http://127.0.0.1:8000/\").raise_for_status()'"]
      interval: 5s
      timeout: 20s
      retries: 10
  mysql:
    image: mysql:8.4
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
      MYSQL_DATABASE: bugsink
      MYSQL_USER: bugsink
      MYSQL_PASSWORD: $SERVICE_PASSWORD_BUGSINK
    volumes:
      - bugsink_mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1"]
      interval: 5s
      timeout: 20s
      retries: 10
`,
	},
	{
		ID:                     "termix",
		Name:                   "Termix",
		Slogan:                 "A web-based SSH, RDP, and VNC terminal manager for organizing and connecting to every server from one dashboard.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/LukeGus/Termix",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  termix:
    image: ghcr.io/lukegus/termix:2.6.1
    ports: ["8080:8080"]
    environment:
      PORT: "8080"
      GUACD_HOST: guacd
      GUACD_RECORDING_PATH: /termix-data/session_recordings/guacamole
    volumes:
      - termix_data:/app/data
    depends_on: [guacd]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8080/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 15s
  guacd:
    image: guacamole/guacd:1.6.0
`,
	},
	{ //nolint:gosec // SIYUAN_ACCESS_AUTH_CODE below is a compose magic-var token, not a real credential
		ID:                     "siyuan",
		Name:                   "SiYuan",
		Slogan:                 "A privacy-first, block-based note-taking app that blends outlining, block references, and a built-in knowledge graph.",
		Category:               "Productivity",
		DocumentationURL:       "https://b3log.org/siyuan/en/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  siyuan:
    image: b3log/siyuan:v3.3.5
    ports: ["6806:6806"]
    environment:
      TZ: UTC
      PUID: "1000"
      PGID: "1000"
      SIYUAN_ACCESS_AUTH_CODE: $SERVICE_PASSWORD_SIYUAN
    volumes:
      - siyuan_workspace:/siyuan/workspace
    healthcheck:
      test: ["CMD", "wget", "--spider", "--quiet", "http://127.0.0.1:6806/api/system/version"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{ //nolint:gosec // SECRET/DB_PASSWORD/POSTGRES_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "yamtrack",
		Name:                   "Yamtrack",
		Slogan:                 "A self-hosted tracker for movies, shows, anime, manga, and games, with watch progress and ratings in one place.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/FuzzyGrim/Yamtrack",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream.
		Compose: `services:
  yamtrack:
    image: ghcr.io/fuzzygrim/yamtrack:latest
    ports: ["8000:8000"]
    environment:
      URLS: ${SERVICE_FQDN_YAMTRACK:-http://localhost:8000}
      TZ: Europe/Berlin
      SECRET: $SERVICE_PASSWORD_SECRET
      REGISTRATION: "true"
      REDIS_URL: redis://redis:6379
      DB_HOST: postgres
      DB_NAME: yamtrack-db
      DB_USER: yamtrack
      DB_PASSWORD: $SERVICE_PASSWORD_POSTGRESQL
      DB_PORT: "5432"
    depends_on: [postgres, redis]
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:8000/health/"]
      interval: 10s
      timeout: 5s
      retries: 10
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: yamtrack
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRESQL
      POSTGRES_DB: yamtrack-db
    volumes:
      - yamtrack_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U yamtrack -d yamtrack-db"]
      interval: 5s
      timeout: 5s
      retries: 10
  redis:
    image: redis:7-alpine
    volumes:
      - yamtrack_redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
}
