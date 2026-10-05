package catalog

// productivityBatchTemplates adds a second wave of well-known
// productivity/collaboration tools to the catalog (ADR 015). Two
// candidates from the original import list were dropped: "getoutline"
// duplicates the already-shipped "outline" entry, and "opnform" needs
// an nginx reverse proxy fed by an inline config file this platform's
// compose subset can't express (no bind-mount file content, see
// templates_applications.go's SearXNG entry for the same limitation).
var productivityBatchTemplates = []Template{
	{
		ID:                     "appflowy",
		Name:                   "AppFlowy",
		Slogan:                 "A self-hosted, open-source workspace for notes and collaborative knowledge, an alternative to Notion.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.appflowy.io/docs/documentation/appflowy-cloud",
		RecommendedMemoryBytes: 1610612736, // 1536Mi
		// Upstream's own compose fronts gotrue/the cloud API/the admin
		// console with an nginx reverse proxy driven by a mounted
		// nginx.conf; this platform's compose subset has no bind-mount
		// file content support, so each backend is reached directly and
		// only the admin console (AppFlowy's only browser UI; the real
		// client is the desktop/mobile app) is exposed.
		Compose: `services:
  postgres:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: appflowy
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: appflowy
    volumes:
      - appflowy_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U appflowy -d appflowy"]
      interval: 5s
      timeout: 5s
      retries: 12
  redis:
    image: redis:7-alpine
    volumes:
      - appflowy_redis_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
      interval: 10s
      timeout: 5s
      retries: 5
  minio:
    image: ghcr.io/coollabsio/minio:RELEASE.2025-10-15T17-29-55Z
    environment:
      MINIO_ROOT_USER: appflowy
      MINIO_ROOT_PASSWORD: $SERVICE_PASSWORD_MINIO
    command: ["server", "/data", "--console-address", ":9001"]
    volumes:
      - appflowy_minio_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://127.0.0.1:9000/minio/health/live"]
      interval: 15s
      timeout: 10s
      retries: 5
  gotrue:
    image: appflowyinc/gotrue:0.9.149
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      GOTRUE_API_HOST: 0.0.0.0
      GOTRUE_API_PORT: "9999"
      GOTRUE_ADMIN_EMAIL: admin@example.com
      GOTRUE_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      GOTRUE_DISABLE_SIGNUP: "false"
      GOTRUE_SITE_URL: "appflowy-flutter://"
      GOTRUE_URI_ALLOW_LIST: "**"
      GOTRUE_JWT_SECRET: $SERVICE_PASSWORD_64_JWT
      GOTRUE_DB_DRIVER: postgres
      API_EXTERNAL_URL: http://gotrue:9999
      DATABASE_URL: postgres://appflowy:$SERVICE_PASSWORD_DB@postgres:5432/appflowy?search_path=auth
      GOTRUE_MAILER_AUTOCONFIRM: "true"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://127.0.0.1:9999/health"]
      interval: 10s
      timeout: 5s
      retries: 10
  appflowy_cloud:
    image: appflowyinc/appflowy_cloud:0.9.149
    depends_on:
      gotrue:
        condition: service_healthy
      redis:
        condition: service_started
    environment:
      RUST_LOG: info
      APPFLOWY_ENVIRONMENT: production
      APPFLOWY_BASE_URL: http://localhost:3000
      APPFLOWY_DATABASE_URL: postgres://appflowy:$SERVICE_PASSWORD_DB@postgres:5432/appflowy
      APPFLOWY_REDIS_URI: redis://redis:6379
      APPFLOWY_GOTRUE_JWT_SECRET: $SERVICE_PASSWORD_64_JWT
      APPFLOWY_GOTRUE_BASE_URL: http://gotrue:9999
      APPFLOWY_S3_USE_MINIO: "true"
      APPFLOWY_S3_CREATE_BUCKET: "true"
      APPFLOWY_S3_MINIO_URL: http://minio:9000
      APPFLOWY_S3_ACCESS_KEY: appflowy
      APPFLOWY_S3_SECRET_KEY: $SERVICE_PASSWORD_MINIO
      APPFLOWY_S3_BUCKET: appflowy
      APPFLOWY_ACCESS_CONTROL: "true"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://127.0.0.1:8000/api/health"]
      interval: 10s
      timeout: 5s
      retries: 12
  admin_frontend:
    image: appflowyinc/admin_frontend:0.9.149
    ports: ["3000:3000"]
    depends_on:
      gotrue:
        condition: service_healthy
      appflowy_cloud:
        condition: service_started
    environment:
      APPFLOWY_GOTRUE_BASE_URL: http://gotrue:9999
      APPFLOWY_BASE_URL: ${SERVICE_FQDN_ADMIN_FRONTEND:-http://localhost:3000}
`,
	},
	{
		ID:                     "joplin",
		Name:                   "Joplin Server",
		Slogan:                 "A self-hosted sync server for the Joplin note-taking app, keeping notes off third-party clouds.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/laurent22/joplin/blob/dev/packages/server/README.md",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: joplin
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: joplin
    volumes:
      - joplin_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U joplin -d joplin"]
      interval: 10s
      timeout: 5s
      retries: 5
  joplin:
    image: joplin/server:latest
    ports: ["22300:22300"]
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      APP_BASE_URL: ${SERVICE_FQDN_JOPLIN:-http://localhost:22300}
      DB_CLIENT: pg
      POSTGRES_USER: joplin
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DATABASE: joplin
      POSTGRES_PORT: "5432"
      POSTGRES_HOST: postgres
    healthcheck:
      test: ["CMD-SHELL", "wget --spider -q http://127.0.0.1:22300/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "triliumnext",
		Name:                   "TriliumNext",
		Slogan:                 "A hierarchical, self-hosted notebook for building a personal knowledge base, with full-text search.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/TriliumNext/Trilium",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  triliumnext:
    image: ghcr.io/triliumnext/trilium:stable
    ports: ["8080:8080"]
    environment:
      TZ: UTC
    volumes:
      - triliumnext_data:/home/node/trilium-data
    healthcheck:
      test: ["CMD-SHELL", "wget --quiet --tries=1 --spider http://127.0.0.1:8080/api/health-check || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{ //nolint:gosec // PRISMA_DATABASE_URL/DATABASE_URL below are compose magic-var tokens ($SERVICE_PASSWORD_DB), not real credentials
		ID:                     "teable",
		Name:                   "Teable",
		Slogan:                 "A spreadsheet-style visual database backed by real PostgreSQL, an Airtable alternative.",
		Category:               "Productivity",
		DocumentationURL:       "https://help.teable.io/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  teable-db:
    image: postgres:15.4
    environment:
      POSTGRES_USER: teable
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: teable
    volumes:
      - teable_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U teable -d teable"]
      interval: 5s
      timeout: 5s
      retries: 10
  teable-cache:
    image: redis:7.2.4
    volumes:
      - teable_cache_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
      interval: 10s
      timeout: 5s
      retries: 5
  teable-db-migrate:
    image: ghcr.io/teableio/teable-db-migrate:latest
    restart: "no"
    depends_on:
      teable-db:
        condition: service_healthy
    environment:
      PRISMA_DATABASE_URL: postgresql://teable:$SERVICE_PASSWORD_DB@teable-db:5432/teable
  teable:
    image: ghcr.io/teableio/teable:latest
    ports: ["3000:3000"]
    depends_on:
      teable-cache:
        condition: service_started
      teable-db:
        condition: service_healthy
    environment:
      PUBLIC_ORIGIN: ${SERVICE_FQDN_TEABLE:-http://localhost:3000}
      PRISMA_DATABASE_URL: postgresql://teable:$SERVICE_PASSWORD_DB@teable-db:5432/teable
      SECRET_KEY: $SERVICE_PASSWORD_64_SECRETKEY
      PORT: "3000"
      BACKEND_CACHE_PROVIDER: redis
      BACKEND_CACHE_REDIS_URI: redis://teable-cache:6379/0
    volumes:
      - teable_data:/app/.assets
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://127.0.0.1:3000"]
      interval: 10s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "nocobase",
		Name:                   "NocoBase",
		Slogan:                 "An extensible no-code/low-code platform for building internal tools and databases.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.nocobase.com/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: nocobase
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: nocobase
    volumes:
      - nocobase_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U nocobase -d nocobase"]
      interval: 5s
      timeout: 5s
      retries: 10
  nocobase:
    image: nocobase/nocobase:1.9.33
    ports: ["13000:13000"]
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      APP_KEY: $SERVICE_BASE64_64_APPKEY
      DB_DIALECT: postgres
      DB_HOST: postgres
      DB_PORT: "5432"
      DB_DATABASE: nocobase
      DB_USER: nocobase
      DB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - nocobase_storage:/app/nocobase/storage
    healthcheck:
      test: ["CMD-SHELL", "wget --spider -q http://127.0.0.1:13000/ || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 10
`,
	},
	{
		ID:                     "plane",
		Name:                   "Plane",
		Slogan:                 "An open-source project management tool for tracking issues and cycles, an alternative to Linear and Jira.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.plane.so/self-hosting/methods/docker-compose",
		RecommendedMemoryBytes: 2147483648, // 2048Mi
		// Upstream's own compose also runs a worker, a beat-worker, a
		// migrator, a space app, an admin app, and a live (realtime)
		// service off the same two backend images; trimmed here to the
		// path that actually serves traffic, the same simplification
		// templates_communication.go's Chatwoot entry makes for its own
		// sidekiq worker.
		Compose: `services:
  plane-db:
    image: postgres:15.7-alpine
    environment:
      POSTGRES_USER: plane
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: plane
    volumes:
      - plane_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U plane -d plane"]
      interval: 5s
      timeout: 5s
      retries: 10
  plane-redis:
    image: valkey/valkey:7.2.11-alpine
    volumes:
      - plane_redis_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
      interval: 10s
      timeout: 5s
      retries: 5
  plane-mq:
    image: rabbitmq:3.13.6-management-alpine
    environment:
      RABBITMQ_DEFAULT_USER: plane
      RABBITMQ_DEFAULT_PASS: $SERVICE_PASSWORD_MQ
      RABBITMQ_DEFAULT_VHOST: plane
    volumes:
      - plane_mq_data:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD-SHELL", "rabbitmq-diagnostics -q ping"]
      interval: 15s
      timeout: 10s
      retries: 5
  plane-minio:
    image: ghcr.io/coollabsio/minio:RELEASE.2025-10-15T17-29-55Z
    environment:
      MINIO_ROOT_USER: plane
      MINIO_ROOT_PASSWORD: $SERVICE_PASSWORD_MINIO
    command: ["server", "/export", "--console-address", ":9090"]
    volumes:
      - plane_uploads:/export
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 10s
      timeout: 10s
      retries: 10
  api:
    image: makeplane/plane-backend:v1.3.0
    command: ["./bin/docker-entrypoint-api.sh"]
    depends_on:
      plane-db:
        condition: service_healthy
      plane-redis:
        condition: service_started
      plane-mq:
        condition: service_started
    environment:
      WEB_URL: http://localhost:8080
      DATABASE_URL: postgresql://plane:$SERVICE_PASSWORD_DB@plane-db/plane
      SECRET_KEY: $SERVICE_PASSWORD_64_SECRETKEY
      AMQP_URL: amqp://plane:$SERVICE_PASSWORD_MQ@plane-mq:5672/plane
      REDIS_URL: redis://plane-redis:6379/
      USE_MINIO: "1"
      MINIO_ENDPOINT_SSL: "0"
      AWS_ACCESS_KEY_ID: plane
      AWS_SECRET_ACCESS_KEY: $SERVICE_PASSWORD_MINIO
      AWS_S3_ENDPOINT_URL: http://plane-minio:9000
      AWS_S3_BUCKET_NAME: uploads
    volumes:
      - plane_logs_api:/code/plane/logs
  web:
    image: makeplane/plane-frontend:v1.3.0
    depends_on:
      - api
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:3000 || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 10
  proxy:
    image: makeplane/plane-proxy:v1.3.0
    ports: ["8080:80"]
    depends_on:
      - web
      - api
    environment:
      APP_DOMAIN: ${SERVICE_FQDN_PLANE:-http://localhost:8080}
      FILE_SIZE_LIMIT: "5242880"
      BUCKET_NAME: uploads
      SITE_ADDRESS: ":80"
    volumes:
      - plane_proxy_config:/config
      - plane_proxy_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://127.0.0.1:80"]
      interval: 10s
      timeout: 10s
      retries: 15
`,
	},
	{
		ID:                     "twenty",
		Name:                   "Twenty CRM",
		Slogan:                 "An open-source CRM you fully control, built to look and feel like a modern spreadsheet.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.twenty.com",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		// Upstream's own compose also runs a "worker" off the same image
		// for background jobs; dropped here, same simplification as
		// templates_communication.go's Chatwoot entry.
		Compose: `services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: twenty
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: twenty
    volumes:
      - twenty_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U twenty -d twenty"]
      interval: 5s
      timeout: 5s
      retries: 10
  redis:
    image: redis:7-alpine
    volumes:
      - twenty_redis_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
      interval: 10s
      timeout: 5s
      retries: 5
  twenty:
    image: twentycrm/twenty:v1.15
    ports: ["3000:3000"]
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started
    environment:
      SERVER_URL: ${SERVICE_FQDN_TWENTY:-http://localhost:3000}
      FRONT_BASE_URL: ${SERVICE_FQDN_TWENTY:-http://localhost:3000}
      APP_SECRET: $SERVICE_BASE64_32_SECRET
      ENABLE_DB_MIGRATIONS: "true"
      CACHE_STORAGE_TYPE: redis
      REDIS_URL: redis://redis:6379
      PG_DATABASE_URL: postgres://twenty:$SERVICE_PASSWORD_DB@postgres:5432/twenty
      STORAGE_TYPE: local
      MESSAGE_QUEUE_TYPE: pg-boss
    volumes:
      - twenty_storage:/app/packages/twenty-server/.local-storage
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://127.0.0.1:3000/healthz"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 30s
`,
	},
	{
		ID:                     "redmine",
		Name:                   "Redmine",
		Slogan:                 "A flexible, mature project management and issue-tracking web application.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.redmine.org/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  postgresql:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: redmine
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: redmine
    volumes:
      - redmine_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U redmine -d redmine"]
      interval: 5s
      timeout: 5s
      retries: 10
  redmine:
    image: redmine:6-alpine
    ports: ["3000:3000"]
    depends_on:
      postgresql:
        condition: service_healthy
    environment:
      SECRET_KEY_BASE: $SERVICE_PASSWORD_64_SECRETKEYBASE
      REDMINE_DB_POSTGRES: postgresql
      REDMINE_DB_PORT: "5432"
      REDMINE_DB_USERNAME: redmine
      REDMINE_DB_PASSWORD: $SERVICE_PASSWORD_DB
      REDMINE_DB_DATABASE: redmine
    volumes:
      - redmine_files:/usr/src/redmine/files
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:3000/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 30s
`,
	},
	{
		ID:                     "moodle",
		Name:                   "Moodle",
		Slogan:                 "A widely used, highly customizable learning management system for online courses.",
		Category:               "Applications",
		DocumentationURL:       "https://moodle.org",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  mariadb:
    image: mariadb:11.1
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
      MYSQL_DATABASE: moodle
      MYSQL_USER: moodle
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - moodle_mariadb_data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 --silent"]
      interval: 10s
      timeout: 5s
      retries: 10
  moodle:
    image: docker.io/bitnamilegacy/moodle:4.3
    ports: ["8080:8080"]
    depends_on:
      - mariadb
    environment:
      MOODLE_DATABASE_HOST: mariadb
      MOODLE_DATABASE_PORT_NUMBER: "3306"
      MOODLE_DATABASE_USER: moodle
      MOODLE_DATABASE_NAME: moodle
      MOODLE_DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
      ALLOW_EMPTY_PASSWORD: "no"
      MOODLE_USERNAME: admin
      MOODLE_PASSWORD: $SERVICE_PASSWORD_ADMIN
      MOODLE_EMAIL: admin@example.com
      MOODLE_SITE_NAME: My Moodle Site
    volumes:
      - moodle_data:/bitnami/moodle
      - moodledata_data:/bitnami/moodledata
    healthcheck:
      test: ["CMD", "php", "-r", "exit(file_exists('/opt/bitnami/moodle/config.php') ? 0 : 1);"]
      interval: 20s
      timeout: 10s
      retries: 10
`,
	},
	{
		ID:                     "limesurvey",
		Name:                   "LimeSurvey",
		Slogan:                 "A mature, self-hosted online survey tool for building and analyzing anonymous surveys.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.limesurvey.org/manual/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  mariadb:
    image: mariadb:11
    environment:
      MYSQL_USER: limesurvey
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_DATABASE: limesurvey
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
    volumes:
      - limesurvey_mariadb_data:/var/lib/mysql
    healthcheck:
      test: ["CMD-SHELL", "mysqladmin ping -h 127.0.0.1 --silent"]
      interval: 10s
      timeout: 5s
      retries: 10
  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
      interval: 10s
      timeout: 5s
      retries: 5
  limesurvey:
    image: adamzammit/limesurvey:latest
    ports: ["8080:80"]
    depends_on:
      mariadb:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      LIMESURVEY_DB_HOST: mariadb
      LIMESURVEY_DB_PASSWORD: $SERVICE_PASSWORD_DB
      LIMESURVEY_DB_USER: limesurvey
      LIMESURVEY_DB_NAME: limesurvey
      LIMESURVEY_ADMIN_USER: admin
      LIMESURVEY_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      LIMESURVEY_ADMIN_NAME: Admin
      LIMESURVEY_ADMIN_EMAIL: admin@example.com
      LIMESURVEY_PHP_SESSION_SAVE_HANDLER: redis
      LIMESURVEY_PHP_SESSION_SAVE_PATH: tcp://redis:6379
    volumes:
      - limesurvey_upload_data:/var/www/html/upload
      - limesurvey_config_data:/var/www/html/application/config
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://127.0.0.1"]
      interval: 10s
      timeout: 10s
      retries: 10
`,
	},
	{
		ID:                     "documenso",
		Name:                   "Documenso",
		Slogan:                 "An open-source document signing platform, a self-hosted alternative to DocuSign.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.documenso.com/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Documenso needs a local signing certificate; this generates a
		// self-signed one on first boot instead of mounting a real cert.
		Compose: `services:
  database:
    image: postgres:17
    environment:
      POSTGRES_USER: documenso
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: documenso
    volumes:
      - documenso_postgresql_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U documenso -d documenso"]
      interval: 5s
      timeout: 5s
      retries: 10
  documenso:
    image: documenso/documenso:v1.12.10
    ports: ["3000:3000"]
    depends_on:
      database:
        condition: service_healthy
    environment:
      NEXTAUTH_URL: ${SERVICE_FQDN_DOCUMENSO:-http://localhost:3000}
      NEXTAUTH_SECRET: $SERVICE_BASE64_AUTHSECRET
      NEXT_PRIVATE_ENCRYPTION_KEY: $SERVICE_BASE64_ENCRYPTIONKEY
      NEXT_PRIVATE_ENCRYPTION_SECONDARY_KEY: $SERVICE_BASE64_SECONDARYKEY
      NEXT_PUBLIC_WEBAPP_URL: ${SERVICE_FQDN_DOCUMENSO:-http://localhost:3000}
      NEXT_PRIVATE_DATABASE_URL: postgresql://documenso:$SERVICE_PASSWORD_DB@database:5432/documenso?schema=public
      NEXT_PRIVATE_DIRECT_DATABASE_URL: postgresql://documenso:$SERVICE_PASSWORD_DB@database:5432/documenso?schema=public
      NEXT_PRIVATE_SIGNING_TRANSPORT: local
      NEXT_PRIVATE_SIGNING_LOCAL_FILE_PATH: /app/certs/cert.p12
      NEXT_PRIVATE_SIGNING_LOCAL_FILE_PASSPHRASE: $SERVICE_PASSWORD_SIGNING
      CERT_VALID_DAYS: "365"
    entrypoint:
      - /bin/sh
      - -c
      - |
        mkdir -p /app/certs
        openssl req -x509 -newkey rsa:2048 -days 365 -nodes \
          -keyout /app/certs/private.key -out /app/certs/certificate.crt \
          -subj "/CN=documenso"
        openssl pkcs12 -export -out /app/certs/cert.p12 \
          -inkey /app/certs/private.key -in /app/certs/certificate.crt \
          -legacy -passout pass:$NEXT_PRIVATE_SIGNING_LOCAL_FILE_PASSPHRASE
        exec ./start.sh
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/ || exit 1"]
      interval: 10s
      timeout: 10s
      retries: 20
`,
	},
	{
		ID:                     "docuseal",
		Name:                   "DocuSeal",
		Slogan:                 "A free, open-source document signing tool, a lighter alternative to DocuSign.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.docuseal.co/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  postgresql:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: docuseal
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: docuseal
    volumes:
      - docuseal_postgresql_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U docuseal -d docuseal"]
      interval: 5s
      timeout: 5s
      retries: 10
  docuseal:
    image: docuseal/docuseal:latest
    ports: ["3000:3000"]
    depends_on:
      postgresql:
        condition: service_healthy
    environment:
      HOST: ${SERVICE_FQDN_DOCUSEAL:-http://localhost:3000}
      DATABASE_URL: postgresql://docuseal:$SERVICE_PASSWORD_DB@postgresql:5432/docuseal
    volumes:
      - docuseal_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://127.0.0.1:3000 || exit 1"]
      interval: 10s
      timeout: 10s
      retries: 10
`,
	},
	{
		ID:                     "fider",
		Name:                   "Fider",
		Slogan:                 "A feedback platform for collecting and prioritizing user feature requests.",
		Category:               "Productivity",
		DocumentationURL:       "https://fider.io",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  database:
    image: postgres:12
    environment:
      POSTGRES_USER: fider
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: fider
    volumes:
      - fider_pg_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U fider -d fider"]
      interval: 5s
      timeout: 5s
      retries: 10
  fider:
    image: getfider/fider:stable
    ports: ["3000:3000"]
    depends_on:
      database:
        condition: service_healthy
    environment:
      BASE_URL: ${SERVICE_FQDN_FIDER:-http://localhost:3000}
      DATABASE_URL: postgres://fider:$SERVICE_PASSWORD_DB@database:5432/fider?sslmode=disable
      JWT_SECRET: $SERVICE_PASSWORD_64_JWTSECRET
      EMAIL_NOREPLY: noreply@example.com
    healthcheck:
      test: ["CMD", "/app/fider", "ping"]
      interval: 10s
      timeout: 5s
      retries: 10
`,
	},
}
