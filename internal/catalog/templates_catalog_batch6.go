package catalog

// catalogBatch6Templates ports a further wave of templates from Coolify's
// public service catalog (ADR 015), adapted to this platform's Compose
// subset: no bind mounts, cap_add, privileged/host networking, or UDP ports.
var catalogBatch6Templates = []Template{
	{
		// LinuxServer publishes only rolling tags for Emby.
		ID:                     "emby",
		Name:                   "Emby",
		Slogan:                 "A media server that organizes your movies, shows, and music and streams them to any device.",
		Category:               "Media",
		DocumentationURL:       "https://emby.media/support/articles/Home.html",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  emby:
    image: lscr.io/linuxserver/emby:latest
    ports: ["8096:8096"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
    volumes:
      - emby_config:/config
      - emby_tvshows:/tvshows
      - emby_movies:/movies
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:8096"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "embystat",
		Name:                   "EmbyStat",
		Slogan:                 "Watch statistics and insight dashboards for your Emby or Jellyfin server.",
		Category:               "Analytics",
		DocumentationURL:       "https://github.com/mregni/EmbyStat",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  embystat:
    image: lscr.io/linuxserver/embystat:latest
    ports: ["6555:6555"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
    volumes:
      - embystat_config:/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:6555"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "budge",
		Name:                   "Budge",
		Slogan:                 "A self-hosted envelope budgeting app for personal finance.",
		Category:               "Finance",
		DocumentationURL:       "https://github.com/linuxserver/budge",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  budge:
    image: lscr.io/linuxserver/budge:latest
    ports: ["80:80"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
    volumes:
      - budge_config:/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:80"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "snapdrop",
		Name:                   "Snapdrop",
		Slogan:                 "Local network file sharing in the browser, inspired by AirDrop.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/RobinLinus/snapdrop",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  snapdrop:
    image: lscr.io/linuxserver/snapdrop:latest
    ports: ["80:80"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
    volumes:
      - snapdrop_config:/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:80"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "firefox-browser",
		Name:                   "Firefox",
		Slogan:                 "A private Firefox browser running on your server and streamed to a web page.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/jlesage/docker-firefox",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  firefox:
    image: jlesage/firefox:latest
    ports: ["5800:5800"]
    volumes:
      - firefox_config:/config
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://127.0.0.1:5800/ || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "lobe-chat",
		Name:                   "Lobe Chat",
		Slogan:                 "A modern, open-source AI chat framework supporting many model providers.",
		Category:               "AI",
		DocumentationURL:       "https://lobehub.com/docs/self-hosting/start",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  lobe-chat:
    image: lobehub/lobe-chat:1.135.5
    ports: ["3210:3210"]
    environment:
      OPENAI_API_KEY: ${OPENAI_API_KEY:-}
      OPENAI_PROXY_URL: ${OPENAI_BASE_URL:-https://api.openai.com/v1}
      ACCESS_CODE: $SERVICE_PASSWORD_ACCESSCODE
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://127.0.0.1:3210/ || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "convertx-app",
		Name:                   "ConvertX",
		Slogan:                 "A self-hosted online file converter supporting over a thousand formats.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/C4illin/ConvertX",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  convertx:
    image: ghcr.io/c4illin/convertx:latest
    ports: ["3000:3000"]
    environment:
      ACCOUNT_REGISTRATION: "false"
      HTTP_ALLOWED: "true"
      AUTO_DELETE_EVERY_N_HOURS: "24"
      JWT_SECRET: $SERVICE_PASSWORD_64_JWT
    volumes:
      - convertx_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://127.0.0.1:3000/ || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "marimo",
		Name:                   "marimo",
		Slogan:                 "A reactive Python notebook that is reproducible, git-friendly, and shareable as an app.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://marimo.io/",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  marimo:
    image: ghcr.io/marimo-team/marimo:latest-sql
    ports: ["8080:8080"]
    command: ["marimo", "edit", "--token-password", "$SERVICE_PASSWORD_MARIMO", "--port", "8080", "--host", "0.0.0.0"]
    volumes:
      - marimo_data:/app
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 8080 || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "proxyscotch",
		Name:                   "Proxyscotch",
		Slogan:                 "A small CORS proxy that lets Hoppscotch call APIs from the browser.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/hoppscotch/proxyscotch",
		RecommendedMemoryBytes: 67108864,
		Compose: `services:
  proxyscotch:
    image: hoppscotch/proxyscotch:v0.1.4
    ports: ["9159:9159"]
    environment:
      PROXYSCOTCH_TOKEN: $SERVICE_PASSWORD_TOKEN
      PROXYSCOTCH_ALLOWED_ORIGINS: "*"
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 9159 || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 5s
`,
	},
	{
		// Probe is a TCP check: the image is minimal and no health route is documented.
		ID:                     "trailbase",
		Name:                   "TrailBase",
		Slogan:                 "A fast Rust and SQLite application server with type-safe APIs and auth.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://trailbase.io/",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  trailbase:
    image: trailbase/trailbase:0.22.6
    ports: ["4000:4000"]
    volumes:
      - trailbase_data:/app/traildepot
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 4000 || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "unstructured",
		Name:                   "Unstructured",
		Slogan:                 "An API that turns PDFs, Word files, and other documents into clean data for RAG pipelines.",
		Category:               "AI",
		DocumentationURL:       "https://github.com/Unstructured-IO/unstructured-api",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  unstructured:
    image: downloads.unstructured.io/unstructured-io/unstructured-api:latest
    ports: ["8000:8000"]
    environment:
      UNSTRUCTURED_API_KEY: $SERVICE_PASSWORD_APIKEY
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8000/healthcheck"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "vvveb",
		Name:                   "Vvveb CMS",
		Slogan:                 "A CMS for building websites, blogs, and ecommerce stores.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.vvveb.com",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  vvveb:
    image: vvveb/vvvebcms:latest
    ports: ["80:80"]
    volumes:
      - vvveb_data:/var/www/html
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{
		ID:                     "chibisafe",
		Name:                   "Chibisafe",
		Slogan:                 "A file vault for uploading and sharing files from the cloud.",
		Category:               "Storage",
		DocumentationURL:       "https://chibisafe.app/docs/intro",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  chibisafe:
    image: chibisafe/chibisafe:v6.5.5
    ports: ["8001:8001"]
    environment:
      BASE_API_URL: http://chibisafe-server:8000
    depends_on:
      - chibisafe-server
    healthcheck:
      test: ["CMD", "wget", "--spider", "--quiet", "http://127.0.0.1:8001"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
  chibisafe-server:
    image: chibisafe/chibisafe-server:v6.5.5
    volumes:
      - chibisafe_database:/app/database
      - chibisafe_uploads:/app/uploads
      - chibisafe_logs:/app/logs
    healthcheck:
      test: ["CMD", "wget", "--spider", "--quiet", "http://127.0.0.1:8000/api/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "cloudreve",
		Name:                   "Cloudreve",
		Slogan:                 "A self-hosted file management and sharing system with multiple storage backends.",
		Category:               "Storage",
		DocumentationURL:       "https://docs.cloudreve.org/",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  cloudreve:
    image: cloudreve/cloudreve:4.10.1
    ports: ["5212:5212"]
    environment:
      CR_CONF_Database.Type: postgres
      CR_CONF_Database.Host: postgres
      CR_CONF_Database.User: cloudreve
      CR_CONF_Database.Password: $SERVICE_PASSWORD_POSTGRES
      CR_CONF_Database.Name: cloudreve
      CR_CONF_Database.Port: "5432"
      CR_CONF_Redis.Server: redis:6379
      CR_CONF_Redis.Password: $SERVICE_PASSWORD_REDIS
    volumes:
      - cloudreve_data:/cloudreve/data
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "nc", "-z", "127.0.0.1", "5212"]
      interval: 20s
      timeout: 10s
      retries: 3
      start_period: 20s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: cloudreve
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: cloudreve
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U cloudreve -d cloudreve"]
      interval: 5s
      timeout: 5s
      retries: 10
  redis:
    image: redis:7-alpine
    command: redis-server --requirepass $SERVICE_PASSWORD_REDIS
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli -a $$SERVICE_PASSWORD_REDIS ping || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "ryot",
		Name:                   "Ryot",
		Slogan:                 "A self-hosted tracker for media, fitness, and other parts of life.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/ignisda/ryot",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  ryot:
    image: ignisda/ryot:v10.3.0
    ports: ["8000:8000"]
    environment:
      DATABASE_URL: postgres://app:$SERVICE_PASSWORD_POSTGRES@postgres:5432/ryot
      SERVER_ADMIN_ACCESS_TOKEN: $SERVICE_PASSWORD_64_ADMIN
      TZ: Etc/UTC
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:8000/health"]
      interval: 15s
      timeout: 20s
      retries: 10
      start_period: 20s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: ryot
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d ryot"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "new-api",
		Name:                   "New API",
		Slogan:                 "An LLM gateway and AI asset management system that unifies many model providers.",
		Category:               "AI",
		DocumentationURL:       "https://docs.newapi.pro/en/getting-started/",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  new-api:
    image: calciumion/new-api:v0.9.2.0
    ports: ["3000:3000"]
    environment:
      SQL_DSN: postgres://app:$SERVICE_PASSWORD_POSTGRES@postgres:5432/newapi?sslmode=disable
      REDIS_CONN_STRING: redis://redis:6379
      TZ: Etc/UTC
      SESSION_SECRET: $SERVICE_PASSWORD_64_SESSION
      ERROR_LOG_ENABLED: "true"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O - http://127.0.0.1:3000/api/status || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 20s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: newapi
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d newapi"]
      interval: 5s
      timeout: 5s
      retries: 10
  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "gramps-web",
		Name:                   "Gramps Web",
		Slogan:                 "An online genealogy system for researching and sharing family trees.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.grampsweb.org/install_setup/setup/",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  grampsweb:
    image: ghcr.io/gramps-project/grampsweb:25.9.0
    ports: ["5000:5000"]
    environment:
      GRAMPSWEB_TREE: Gramps Web
      GRAMPSWEB_SECRET_KEY: $SERVICE_PASSWORD_64_SECRET
      GRAMPSWEB_CELERY_CONFIG__broker_url: redis://grampsweb_redis:6379/0
      GRAMPSWEB_CELERY_CONFIG__result_backend: redis://grampsweb_redis:6379/0
      GRAMPSWEB_RATELIMIT_STORAGE_URI: redis://grampsweb_redis:6379/1
      GUNICORN_NUM_WORKERS: "2"
    depends_on:
      - grampsweb_redis
    volumes:
      - gramps_users:/app/users
      - gramps_index:/app/indexdir
      - gramps_thumb_cache:/app/thumbnail_cache
      - gramps_cache:/app/cache
      - gramps_secret:/app/secret
      - gramps_db:/root/.gramps/grampsdb
      - gramps_media:/app/media
      - gramps_tmp:/tmp
    healthcheck:
      test: ["CMD-SHELL", "wget -O - http://127.0.0.1:5000 > /dev/null 2>&1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  grampsweb_redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep PONG"]
      interval: 5s
      timeout: 5s
      retries: 5
`,
	},
	{
		// Set the API token and domains before deploy.
		ID:                     "cloudflare-ddns",
		Name:                   "Cloudflare DDNS",
		Slogan:                 "Keeps Cloudflare DNS records pointed at your current public IP address.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://github.com/favonia/cloudflare-ddns",
		RecommendedMemoryBytes: 33554432,
		Compose: `services:
  cloudflare-ddns:
    image: favonia/cloudflare-ddns:1.15.1
    environment:
      CLOUDFLARE_API_TOKEN: ${CLOUDFLARE_API_TOKEN:-}
      DOMAINS: ${DOMAINS:-}
      PROXIED: "false"
    healthcheck:
      test: ["CMD", "/bin/ddns", "--help"]
      interval: 60s
      timeout: 10s
      retries: 3
`,
	},
	{ //nolint:gosec // credential-looking env vars below are compose magic-var tokens, not real credentials
		ID:                     "sparkyfitness",
		Name:                   "SparkyFitness",
		Slogan:                 "A fitness tracker for nutrition, exercise, and body measurements.",
		Category:               "Productivity",
		DocumentationURL:       "https://codewithcj.github.io/SparkyFitness/",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  sparkyfitness-frontend:
    image: codewithcj/sparkyfitness:v1.6.1
    ports: ["8080:80"]
    environment:
      SPARKY_FITNESS_SERVER_HOST: sparkyfitness-server
      SPARKY_FITNESS_SERVER_PORT: "3010"
    depends_on:
      - sparkyfitness-server
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://127.0.0.1:80 || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 15s
  sparkyfitness-server:
    image: codewithcj/sparkyfitness_server:v1.6.1
    environment:
      SPARKY_FITNESS_LOG_LEVEL: info
      NODE_ENV: production
      TZ: Etc/UTC
      SPARKY_FITNESS_DB_USER: app
      SPARKY_FITNESS_DB_HOST: postgres
      SPARKY_FITNESS_DB_NAME: sparkyfitness
      SPARKY_FITNESS_DB_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      SPARKY_FITNESS_DB_PORT: "5432"
      SPARKY_FITNESS_APP_DB_USER: sparkyapp
      SPARKY_FITNESS_APP_DB_PASSWORD: $SERVICE_PASSWORD_64_APPDB
      SPARKY_FITNESS_API_ENCRYPTION_KEY: $SERVICE_PASSWORD_64_ENCRYPTION
      BETTER_AUTH_SECRET: $SERVICE_PASSWORD_64_AUTH
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 3010 || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 10
      start_period: 30s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: sparkyfitness
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d sparkyfitness"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
}
