package catalog

// catalogBatch5Templates ports a fifth wave of well-known templates from
// Coolify's own public service catalog (ADR 015), adapted the same way
// batch 4 was: no bind-mount config injection, no cap_add/sysctls, no
// privileged/network_mode, no UDP ports. Azimutt, Garage, and OpnForm
// were left out: each needs a bind-mounted generated config file (SMTP
// relay/MinIO setup, garage.toml, and nginx.conf respectively) that this
// platform's Compose subset has no env-var-only equivalent for.
var catalogBatch5Templates = []Template{
	{
		ID:                     "elasticsearch",
		Name:                   "Elasticsearch",
		Slogan:                 "A distributed, RESTful search and analytics engine for full-text search and log analytics.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://www.elastic.co/guide/en/elasticsearch/reference/current/index.html",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		// Security left off: this platform's readiness probe can't send
		// basic-auth credentials, and 8.x enables security by default.
		// Put a real auth boundary in front before exposing it publicly.
		Compose: `services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.19.22
    ports: ["9200:9200"]
    environment:
      discovery.type: single-node
      xpack.security.enabled: "false"
      ES_JAVA_OPTS: "-Xms512m -Xmx512m"
    volumes:
      - elasticsearch_data:/usr/share/elasticsearch/data
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:9200/_cluster/health"]
      interval: 15s
      timeout: 10s
      retries: 10
      start_period: 30s
`,
	},
	{
		ID:                     "glpi",
		Name:                   "GLPI",
		Slogan:                 "An IT asset and service management platform with helpdesk ticketing built in.",
		Category:               "Applications",
		DocumentationURL:       "https://glpi-project.org/documentation/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  glpi:
    image: glpi/glpi:11.0.11
    ports: ["8080:80"]
    environment:
      GLPI_DB_HOST: glpi-db
      GLPI_DB_NAME: glpi
      GLPI_DB_USER: glpi
      GLPI_DB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - glpi_data:/var/glpi
    depends_on: [glpi-db]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  glpi-db:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: glpi
      MYSQL_USER: glpi
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_64_DBROOT
    volumes:
      - glpi_mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "browserless",
		Name:                   "Browserless",
		Slogan:                 "A headless Chrome browser exposed as an HTTP/WebSocket API for scraping and PDF rendering.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.browserless.io",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  browserless:
    image: ghcr.io/browserless/chromium:v2.8.0
    ports: ["3000:3000"]
    environment:
      TOKEN: $SERVICE_PASSWORD_BROWSERLESS
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:3000/docs"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 15s
`,
	},
	{
		ID:                     "cryptgeon",
		Name:                   "Cryptgeon",
		Slogan:                 "A self-destructing note and file sharing service inspired by PrivNote, with end-to-end encryption.",
		Category:               "Security",
		DocumentationURL:       "https://github.com/cupcakearmy/cryptgeon",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  cryptgeon:
    image: cupcakearmy/cryptgeon:2.9.3
    ports: ["8000:8000"]
    environment:
      SIZE_LIMIT: "4 MiB"
      MAX_VIEWS: "100"
      MAX_EXPIRATION: "360"
    depends_on: [redis]
    healthcheck:
      test: ["CMD", "curl", "--fail", "http://127.0.0.1:8000/api/live/"]
      interval: 30s
      timeout: 5s
      retries: 5
      start_period: 10s
  redis:
    image: redis:7-alpine
    command: ["redis-server", "--maxmemory", "200mb", "--maxmemory-policy", "allkeys-lru"]
    healthcheck:
      test: ["CMD", "redis-cli", "PING"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "easyappointments",
		Name:                   "Easy!Appointments",
		Slogan:                 "An open-source appointment scheduler for managing bookings, staff, and services.",
		Category:               "Productivity",
		DocumentationURL:       "https://easyappointments.org/docs.html",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  easyappointments:
    image: alextselegidis/easyappointments:1.6.0
    ports: ["8000:80"]
    environment:
      BASE_URL: ${SERVICE_FQDN_EASYAPPOINTMENTS:-http://localhost:8000}
      DB_HOST: mysql
      DB_NAME: easyappointments
      DB_USERNAME: root
      DB_PASSWORD: $SERVICE_PASSWORD_DB
    depends_on: [mysql]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1"]
      interval: 15s
      timeout: 10s
      retries: 10
      start_period: 30s
  mysql:
    image: mysql:8.4
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_DATABASE: easyappointments
    volumes:
      - easyappointments_mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "yamtrack",
		Name:                   "Yamtrack",
		Slogan:                 "A self-hosted media tracker for movies, TV, anime, manga, games, and books.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/FuzzyGrim/Yamtrack/wiki",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  yamtrack:
    image: ghcr.io/fuzzygrim/yamtrack:0.26.3
    ports: ["8000:8000"]
    environment:
      URLS: ${SERVICE_FQDN_YAMTRACK:-http://localhost:8000}
      SECRET: $SERVICE_PASSWORD_64_SECRET
      REDIS_URL: redis://redis:6379
    depends_on: [redis]
    volumes:
      - yamtrack_data:/yamtrack/db
    healthcheck:
      test: ["CMD-SHELL", "wget --quiet --tries=1 --spider http://127.0.0.1:8000/health/"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 15s
  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "slash",
		Name:                   "Slash",
		Slogan:                 "A self-hosted link shortener and bookmark sharing platform with tags and full-text search.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/yourselfhosted/slash",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  slash:
    image: yourselfhosted/slash:0.5.3
    ports: ["5231:5231"]
    volumes:
      - slash_data:/var/opt/slash
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://127.0.0.1:5231"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "imgcompress",
		Name:                   "ImgCompress",
		Slogan:                 "An offline image compression, format conversion, and background-removal API for self-hosted pipelines.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://imgcompress.karimzouine.com",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  imgcompress:
    image: karimz1/imgcompress:0.9.0
    ports: ["5000:5000"]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:5000"]
      interval: 15s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "jupyter-notebook-python",
		Name:                   "Jupyter Notebook",
		Slogan:                 "A web-based notebook environment for interactive Python, data analysis, and visualization.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://jupyter.org/documentation",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  jupyter:
    image: quay.io/jupyter/base-notebook:2026-09-29
    ports: ["8888:8888"]
    command: ["start-notebook.sh"]
    environment:
      JUPYTER_TOKEN: $SERVICE_PASSWORD_JUPYTER
    volumes:
      - jupyter_work:/home/jovyan/work
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8888/"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{
		// single_node and [cors] tuning in upstream's own setup guide need
		// a bind-mounted local.ini, unsupported here: admin auth alone is
		// enough for the plugin's own sync API, add a CORS-aware proxy
		// rule before relying on the browser-based Fauxton UI.
		ID:                     "obsidian-livesync",
		Name:                   "Obsidian LiveSync (CouchDB)",
		Slogan:                 "A self-hosted CouchDB backend for the Obsidian LiveSync plugin, syncing your notes across devices.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/vrtmrz/obsidian-livesync/blob/main/docs/setup_own_server.md",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  couchdb:
    image: couchdb:3.5.2
    ports: ["5984:5984"]
    environment:
      COUCHDB_USER: $SERVICE_USER_COUCHDB
      COUCHDB_PASSWORD: $SERVICE_PASSWORD_64_COUCHDB
    volumes:
      - couchdb_data:/opt/couchdb/data
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:5984/_up"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{
		// Only published under a rolling "main" tag upstream; no pinned
		// release tag exists on this image's registry.
		ID:                     "once-campfire",
		Name:                   "Once Campfire",
		Slogan:                 "A simple, self-hosted group chat app from 37signals, no subscription required.",
		Category:               "Communication",
		DocumentationURL:       "https://github.com/basecamp/once-campfire",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  campfire:
    image: ghcr.io/basecamp/once-campfire:main
    ports: ["80:80"]
    environment:
      SECRET_KEY_BASE: $SERVICE_BASE64_64_CAMPFIRE
      DISABLE_SSL: "true"
      SKIP_TELEMETRY: "true"
    volumes:
      - campfire_storage:/rails/storage
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1/up"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 20s
`,
	},
	{ //nolint:gosec // PBW_POSTGRES_CONN_STRING below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "pgbackweb",
		Name:                   "PG Back Web",
		Slogan:                 "A web UI for scheduling, encrypting, and restoring PostgreSQL backups.",
		Category:               "Database Tools",
		DocumentationURL:       "https://github.com/eduardolat/pgbackweb",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  pgbackweb:
    image: eduardolat/pgbackweb:0.5.2
    ports: ["8085:8085"]
    environment:
      PBW_ENCRYPTION_KEY: $SERVICE_PASSWORD_64_PGBACKWEB
      PBW_POSTGRES_CONN_STRING: postgresql://pgbackweb:$SERVICE_PASSWORD_DB@pgbackweb-db:5432/pgbackweb?sslmode=disable
    volumes:
      - pgbackweb_data:/backups
    depends_on: [pgbackweb-db]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:8085"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 15s
  pgbackweb-db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: pgbackweb
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: pgbackweb
    volumes:
      - pgbackweb_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U pgbackweb -d pgbackweb"]
      interval: 5s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "web-check",
		Name:                   "Web Check",
		Slogan:                 "An all-in-one OSINT tool for inspecting a website's DNS, headers, certs, and security posture.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/lissy93/web-check",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  web-check:
    image: lissy93/web-check:2.3.0
    ports: ["3000:3000"]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 15s
`,
	},
	{
		ID:                     "siyuan",
		Name:                   "SiYuan",
		Slogan:                 "A privacy-first, self-hosted personal knowledge management app with block-based markdown notes.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/siyuan-note/siyuan",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  siyuan:
    image: b3log/siyuan:v3.8.6
    ports: ["6806:6806"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
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
}
