package catalog

var selfhosted4Templates = []Template{
	{
		ID:                     "karakeep",
		Name:                   "Karakeep",
		Slogan:                 "A self-hosted bookmark, note, and read-it-later manager with full-text search and tagging.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.karakeep.app",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// The optional headless-Chrome sidecar (full-page screenshots) and
		// OpenAI-powered auto-tagging are both left out: neither is
		// required for bookmarking, search, or the reading view to work.
		Compose: `services:
  karakeep:
    image: ghcr.io/karakeep-app/karakeep:0.33.2
    ports: ["3000:3000"]
    environment:
      MEILI_ADDR: http://meilisearch:7700
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MEILI
      NEXTAUTH_SECRET: $SERVICE_HEX_32_NEXTAUTH
      NEXTAUTH_URL: http://localhost:3000
      DATA_DIR: /data
    volumes:
      - karakeep_data:/data
    depends_on: [meilisearch]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
  meilisearch:
    image: getmeili/meilisearch:v1.15.0
    environment:
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MEILI
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - karakeep_meili_data:/meili_data
`,
	},
	{
		ID:                     "flaresolverr",
		Name:                   "FlareSolverr",
		Slogan:                 "A headless-browser proxy that solves Cloudflare and DDoS-Guard challenges for other self-hosted tools.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/FlareSolverr/FlareSolverr",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  flaresolverr:
    image: ghcr.io/flaresolverr/flaresolverr:v3.5.0
    ports: ["8191:8191"]
    environment:
      LOG_LEVEL: info
      TZ: UTC
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8191/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "autobrr",
		Name:                   "autobrr",
		Slogan:                 "Matches torrent releases from your indexers against filters and pushes hits straight to your download client.",
		Category:               "Automation",
		DocumentationURL:       "https://autobrr.com/installation/docker",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  autobrr:
    image: ghcr.io/autobrr/autobrr:v1.87.0
    ports: ["7474:7474"]
    environment:
      AUTOBRR__HOST: 0.0.0.0
      AUTOBRR__PORT: "7474"
      AUTOBRR__SESSION_SECRET: $SERVICE_HEX_32_SESSION
    volumes:
      - autobrr_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:7474/api/healthz/liveness || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "kopia",
		Name:                   "Kopia",
		Slogan:                 "Fast, incremental, encrypted backups to a repository, driven from a web UI instead of a cron script.",
		Category:               "Storage",
		DocumentationURL:       "https://kopia.io/docs/installation/#quick-setup-using-docker",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Connects to (or creates, on first boot) a repository on a local
		// volume rather than a cloud target, so this works with no extra
		// credentials. The server's self-signed cert is why the
		// healthcheck curls with -k.
		Compose: `services:
  kopia:
    image: kopia/kopia:0.19.0
    ports: ["51515:51515"]
    environment:
      KOPIA_PASSWORD: $SERVICE_PASSWORD_REPO
      KOPIA_SERVER_USERNAME: admin
      KOPIA_SERVER_PASSWORD: $SERVICE_PASSWORD_ADMIN
    command: ["/bin/sh", "-c", "kopia repository connect filesystem --path=/app/repository || kopia repository create filesystem --path=/app/repository; exec kopia server start --insecure --tls-generate-cert --address=0.0.0.0:51515 --server-username=$KOPIA_SERVER_USERNAME --server-password=$KOPIA_SERVER_PASSWORD"]
    volumes:
      - kopia_repository:/app/repository
      - kopia_config:/app/config
      - kopia_cache:/app/cache
      - kopia_logs:/app/logs
    healthcheck:
      test: ["CMD-SHELL", "curl -fsSk -o /dev/null https://127.0.0.1:51515/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "pingvin-share",
		Name:                   "Pingvin Share",
		Slogan:                 "A self-hosted, ad-free alternative to WeTransfer for sending files with expiring links.",
		Category:               "Storage",
		DocumentationURL:       "https://github.com/stonith404/pingvin-share",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  pingvin-share:
    image: stonith404/pingvin-share:v1.13.0
    ports: ["3000:3000"]
    environment:
      JWT_SECRET: $SERVICE_HEX_32_JWT
      CORS_ALLOW_ORIGIN: http://localhost:3000
    volumes:
      - pingvin_data:/opt/app/backend/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{ //nolint:gosec // MYSQL_PORT_3306_TCP_ADDR/APP_KEY below are compose magic-var tokens and a service hostname, not real credentials
		ID:                     "snipe-it",
		Name:                   "Snipe-IT",
		Slogan:                 "IT asset management for tracking hardware, licenses, and accessories, and who currently has what.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://snipe-it.readme.io/docs/docker",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// APP_KEY needs the "base64:" prefix Laravel expects; the
		// magic-var token only fills in the random portion after it.
		Compose: `services:
  snipeit:
    image: snipe/snipe-it:v7.1.11
    ports: ["8080:80"]
    environment:
      APP_KEY: base64:$SERVICE_BASE64_32_APPKEY
      APP_URL: http://localhost:8080
      MYSQL_DATABASE: snipeit
      MYSQL_USER: snipeit
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_PORT_3306_TCP_ADDR: db
      MYSQL_PORT_3306_TCP_PORT: "3306"
    volumes:
      - snipeit_data:/var/lib/snipeit
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: snipeit
      MYSQL_USER: snipeit
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_RANDOM_ROOT_PASSWORD: "true"
    volumes:
      - snipeit_db_data:/var/lib/mysql
`,
	},
	{
		ID:                     "speedtest-tracker",
		Name:                   "Speedtest Tracker",
		Slogan:                 "Runs Ookla speed tests on a schedule and charts your connection's throughput, latency, and jitter over time.",
		Category:               "Monitoring",
		DocumentationURL:       "https://docs.speedtest-tracker.dev",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  speedtest-tracker:
    image: lscr.io/linuxserver/speedtest-tracker:1.14.1-ls151
    ports: ["8765:80"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: UTC
      APP_KEY: base64:$SERVICE_BASE64_32_APPKEY
      DB_CONNECTION: sqlite
      SPEEDTEST_SCHEDULE: "0 * * * *"
    volumes:
      - speedtest_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/api/healthcheck || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "technitium-dns",
		Name:                   "Technitium DNS Server",
		Slogan:                 "A full-featured authoritative and recursive DNS server with DNS-over-HTTPS/TLS and a web console.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://technitium.com/dns/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Real deployments also expose DNS on 53/tcp+udp; this platform
		// tracks a single container port per service, so only the web
		// console is reachable here, same limitation as this catalog's
		// existing pi-hole and adguard-home entries.
		Compose: `services:
  technitium-dns:
    image: technitium/dns-server:15.5.1
    ports: ["5380:5380"]
    environment:
      DNS_SERVER_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      DNS_SERVER_DOMAIN: dns.localhost
    volumes:
      - technitium_config:/etc/dns
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5380/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "drawio",
		Name:                   "draw.io",
		Slogan:                 "A self-hosted diagramming and whiteboarding editor for flowcharts, architecture diagrams, and more.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.drawio.com/doc/faq/docker",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  drawio:
    image: jgraph/drawio:29.0.3
    ports: ["8080:8080"]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "postiz",
		Name:                   "Postiz",
		Slogan:                 "Schedules and publishes posts across social platforms from one calendar, with basic analytics.",
		Category:               "Communication",
		DocumentationURL:       "https://docs.postiz.com/installation/docker",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  postiz:
    image: ghcr.io/gitroomhq/postiz-app:v2.23.0
    ports: ["5000:5000"]
    environment:
      MAIN_URL: http://localhost:5000
      FRONTEND_URL: http://localhost:5000
      NEXT_PUBLIC_BACKEND_URL: http://localhost:5000/api
      JWT_SECRET: $SERVICE_HEX_32_JWT
      DATABASE_URL: postgresql://postiz:$SERVICE_PASSWORD_DB@db:5432/postiz
      REDIS_URL: redis://redis:6379
      IS_GENERAL: "true"
    volumes:
      - postiz_uploads:/config/uploads
    depends_on: [db, redis]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5000/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: postiz
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: postiz
    volumes:
      - postiz_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postiz -d postiz"]
      interval: 10s
      timeout: 5s
      retries: 5
  redis:
    image: redis:7-alpine
    volumes:
      - postiz_redis_data:/data
`,
	},
	{
		ID:                     "stalwart",
		Name:                   "Stalwart Mail Server",
		Slogan:                 "An all-in-one SMTP, IMAP, JMAP, and WebDAV mail server in a single lightweight container.",
		Category:               "Communication",
		DocumentationURL:       "https://stalw.art/docs/install/platform/docker/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// First boot generates an admin password and prints it to the
		// container logs; sign in at the management console to finish
		// domain and mailbox setup. Only the management/JMAP HTTP port is
		// reachable here; real mail transport ports (25, 143, 465, 587,
		// 993) aren't exposed by this platform's single-port-per-service
		// model.
		Compose: `services:
  stalwart:
    image: stalwartlabs/stalwart:v0.16.23
    ports: ["8080:8080"]
    volumes:
      - stalwart_config:/opt/stalwart/etc
      - stalwart_data:/opt/stalwart/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/healthz/live || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "tdarr",
		Name:                   "Tdarr",
		Slogan:                 "Automated media transcoding, health checks, and library-wide format standardization.",
		Category:               "Media",
		DocumentationURL:       "https://docs.tdarr.io",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		// Runs the combined server+node image in CPU-only mode; GPU
		// transcoding needs a separate node container with a device
		// reservation, not set up here.
		Compose: `services:
  tdarr:
    image: ghcr.io/haveagitgat/tdarr:2.86.01
    ports: ["8265:8265"]
    environment:
      serverIP: 0.0.0.0
      serverPort: "8266"
      webUIPort: "8265"
      internalNode: "true"
      inContainer: "true"
      ffmpegVersion: "6"
      PUID: "1000"
      PGID: "1000"
      TZ: UTC
    volumes:
      - tdarr_server:/app/server
      - tdarr_configs:/app/configs
      - tdarr_logs:/app/logs
      - tdarr_media:/media
      - tdarr_transcode_cache:/temp
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8265/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "photoview",
		Name:                   "Photoview",
		Slogan:                 "A fast, simple photo gallery that indexes an existing folder tree without importing or duplicating files.",
		Category:               "Media",
		DocumentationURL:       "https://photoview.github.io/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  photoview:
    image: photoview/photoview:2.4.0
    ports: ["8080:80"]
    environment:
      PHOTOVIEW_DATABASE_DRIVER: sqlite
      PHOTOVIEW_SQLITE_PATH: /data/photoview.db
      PHOTOVIEW_LISTEN_IP: 0.0.0.0
      PHOTOVIEW_LISTEN_PORT: "80"
    volumes:
      - photoview_data:/data
      - photoview_photos:/photos:ro
      - photoview_cache:/app/cache
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "zabbix",
		Name:                   "Zabbix",
		Slogan:                 "Enterprise-grade network, server, and application monitoring with alerting and trend graphs.",
		Category:               "Monitoring",
		DocumentationURL:       "https://www.zabbix.com/documentation/current/en/manual/installation/containers",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		// Zabbix's own images use a rolling per-branch tag, not this
		// catalog's usual pinned-version convention: alpine-7.4-latest is
		// a real tag from their own registry that tracks the newest 7.4.x
		// patch. The zabbix-agent container (for monitoring the host
		// Zabbix itself runs on) is left out; add it separately if wanted.
		Compose: `services:
  zabbix-server:
    image: zabbix/zabbix-server-pgsql:alpine-7.4-latest
    environment:
      DB_SERVER_HOST: db
      POSTGRES_USER: zabbix
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: zabbix
    volumes:
      - zabbix_server_data:/var/lib/zabbix
    depends_on: [db]
  zabbix-web:
    image: zabbix/zabbix-web-nginx-pgsql:alpine-7.4-latest
    ports: ["8080:8080"]
    environment:
      ZBX_SERVER_HOST: zabbix-server
      DB_SERVER_HOST: db
      POSTGRES_USER: zabbix
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: zabbix
      PHP_TZ: UTC
    depends_on: [zabbix-server, db]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: zabbix
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: zabbix
    volumes:
      - zabbix_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U zabbix -d zabbix"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "netbox",
		Name:                   "NetBox",
		Slogan:                 "Source-of-truth IPAM and DCIM for tracking IP space, racks, devices, and cabling.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://github.com/netbox-community/netbox-docker",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		// A minimal single-process deploy: the separate netbox-worker and
		// netbox-housekeeping containers real installs also run
		// (background jobs, scheduled cleanup) are left out, so those two
		// features won't run here. One redis instance backs both the
		// task queue and the cache, split by database index.
		Compose: `services:
  netbox:
    image: netboxcommunity/netbox:v4.6-5.0.2
    ports: ["8080:8080"]
    environment:
      SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      DB_HOST: db
      DB_NAME: netbox
      DB_USER: netbox
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      REDIS_HOST: redis
      REDIS_PORT: "6379"
      REDIS_DATABASE: "0"
      REDIS_CACHE_HOST: redis
      REDIS_CACHE_PORT: "6379"
      REDIS_CACHE_DATABASE: "1"
      SUPERUSER_NAME: admin
      SUPERUSER_EMAIL: admin@example.com
      SUPERUSER_PASSWORD: $SERVICE_PASSWORD_ADMIN
      ALLOWED_HOSTS: "*"
    volumes:
      - netbox_media:/opt/netbox/netbox/media
      - netbox_reports:/opt/netbox/netbox/reports
      - netbox_scripts:/opt/netbox/netbox/scripts
    depends_on: [db, redis]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/login/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 5
      start_period: 90s
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: netbox
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: netbox
    volumes:
      - netbox_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U netbox -d netbox"]
      interval: 10s
      timeout: 5s
      retries: 5
  redis:
    image: redis:7-alpine
    volumes:
      - netbox_redis_data:/data
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "affine",
		Name:                   "AFFiNE",
		Slogan:                 "A block-based workspace combining docs, whiteboards, and databases in one self-hosted app.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.affine.pro/self-host-affine/references/docker-compose-yml",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  affine:
    image: ghcr.io/toeverything/affine:stable
    ports: ["3010:3010"]
    environment:
      NODE_ENV: production
      AFFINE_SERVER_HOST: localhost
      AFFINE_SERVER_PORT: "3010"
      AFFINE_SERVER_HTTPS: "false"
      DATABASE_URL: postgresql://affine:$SERVICE_PASSWORD_DB@db:5432/affine
      REDIS_SERVER_HOST: redis
    volumes:
      - affine_storage:/root/.affine/storage
      - affine_config:/root/.affine/config
    depends_on: [db, redis]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3010/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: affine
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: affine
    volumes:
      - affine_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U affine -d affine"]
      interval: 10s
      timeout: 5s
      retries: 5
  redis:
    image: redis:7-alpine
    volumes:
      - affine_redis_data:/data
`,
	},
}
