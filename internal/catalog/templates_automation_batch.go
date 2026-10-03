package catalog

// automationBatchTemplates ports a second wave of well-known templates
// from Coolify's own public service catalog (ADR 015), each adapted to
// this platform's narrower Compose subset: no bind-mount config
// injection, no cap_add/sysctls, no volumes_from, no UDP ports. Several
// upstream multi-service templates were simplified or dropped outright
// where the gap had no real translation (see the PR description).
var automationBatchTemplates = []Template{
	{
		ID:                     "n8n-postgres",
		Name:                   "n8n (Postgres)",
		Slogan:                 "n8n backed by Postgres instead of its default SQLite file, for production workflow volumes.",
		Category:               "Automation",
		DocumentationURL:       "https://docs.n8n.io/hosting/installation/server-setups/postgresql/",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  n8n:
    image: n8nio/n8n:2.10.2
    ports: ["5678:5678"]
    environment:
      N8N_ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
      N8N_PROTOCOL: https
      N8N_PORT: "5678"
      WEBHOOK_URL: ${SERVICE_FQDN_N8N:-http://localhost:5678}
      DB_TYPE: postgresdb
      DB_POSTGRESDB_HOST: postgres
      DB_POSTGRESDB_PORT: "5432"
      DB_POSTGRESDB_DATABASE: n8n
      DB_POSTGRESDB_USER: n8n
      DB_POSTGRESDB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - n8n_postgres_data:/home/node/.n8n
    depends_on: [postgres]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:5678/healthz || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: n8n
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: n8n
    volumes:
      - n8n_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U n8n -d n8n"]
      interval: 5s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "flowise-postgres",
		Name:                   "Flowise (Postgres + Qdrant)",
		Slogan:                 "Flowise with a Postgres record manager, a Redis cache, and a Qdrant vector store, for production RAG flows.",
		Category:               "AI",
		DocumentationURL:       "https://docs.flowiseai.com",
		RecommendedMemoryBytes: 2147483648, // 2Gi
		Compose: `services:
  flowise:
    image: flowiseai/flowise:3.1.4
    ports: ["3000:3000"]
    environment:
      PORT: "3000"
      FLOWISE_USERNAME: $SERVICE_USER_FLOWISE
      FLOWISE_PASSWORD: $SERVICE_PASSWORD_FLOWISE
      DATABASE_PATH: /root/.flowise
      APIKEY_PATH: /root/.flowise
      SECRETKEY_PATH: /root/.flowise
      LOG_PATH: /root/.flowise/logs
      BLOB_STORAGE_PATH: /root/.flowise/storage
    volumes:
      - flowise_pg_data:/root/.flowise
    depends_on: [recordmanager, cache, qdrant]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/v1/ping || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  recordmanager:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: flowise
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_RECORDMANAGER
      POSTGRES_DB: flowise_records
    volumes:
      - flowise_pg_recordmanager_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U flowise -d flowise_records"]
      interval: 5s
      timeout: 5s
      retries: 5
  cache:
    image: redis:7-alpine
    volumes:
      - flowise_pg_redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5
  qdrant:
    image: qdrant/qdrant:v1.19.1
    environment:
      QDRANT__SERVICE__API_KEY: $SERVICE_PASSWORD_QDRANT
    volumes:
      - flowise_pg_qdrant_data:/qdrant/storage
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:6333/healthz || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		// budibase.docker.scarf.sh is budibase's own self-hosted
		// distribution channel (a scarf.sh analytics proxy in front of
		// the real images); self-hosters pin :latest and rely on an
		// external updater by upstream's own convention, so that is
		// kept here rather than guessing a version tag that does not
		// exist in this channel.
		ID:                     "budibase",
		Name:                   "Budibase",
		Slogan:                 "An open-source low-code platform for building internal tools, forms, and admin panels.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.budibase.com/docs/docker-compose",
		RecommendedMemoryBytes: 2147483648, // 2Gi
		Compose: `services:
  app-service:
    image: budibase.docker.scarf.sh/budibase/apps:latest
    environment:
      SELF_HOSTED: "1"
      COUCH_DB_URL: http://$SERVICE_USER_COUCHDB:$SERVICE_PASSWORD_COUCHDB@couchdb-service:5984
      WORKER_URL: http://worker-service:4003
      MINIO_URL: http://minio-service:9000
      MINIO_ACCESS_KEY: $SERVICE_USER_MINIO
      MINIO_SECRET_KEY: $SERVICE_PASSWORD_MINIO
      INTERNAL_API_KEY: $SERVICE_BASE64_128_BUDIBASE
      BUDIBASE_ENVIRONMENT: PRODUCTION
      PORT: "4002"
      API_ENCRYPTION_KEY: $SERVICE_BASE64_64_BUDIBASEAPI
      ENCRYPTION_KEY: $SERVICE_BASE64_64_BUDIBASE
      JWT_SECRET: $SERVICE_BASE64_64_BUDIBASEJWT
      LOG_LEVEL: info
      REDIS_URL: redis-service:6379
    depends_on: [worker-service, redis-service]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:4002/health || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  worker-service:
    image: budibase.docker.scarf.sh/budibase/worker:latest
    environment:
      SELF_HOSTED: "1"
      PORT: "4003"
      CLUSTER_PORT: "10000"
      API_ENCRYPTION_KEY: $SERVICE_BASE64_64_BUDIBASEAPI
      JWT_SECRET: $SERVICE_BASE64_64_BUDIBASEJWT
      MINIO_ACCESS_KEY: $SERVICE_USER_MINIO
      MINIO_SECRET_KEY: $SERVICE_PASSWORD_MINIO
      MINIO_URL: http://minio-service:9000
      APPS_URL: http://app-service:4002
      COUCH_DB_USERNAME: $SERVICE_USER_COUCHDB
      COUCH_DB_PASSWORD: $SERVICE_PASSWORD_COUCHDB
      COUCH_DB_URL: http://$SERVICE_USER_COUCHDB:$SERVICE_PASSWORD_COUCHDB@couchdb-service:5984
      INTERNAL_API_KEY: $SERVICE_BASE64_128_BUDIBASE
      REDIS_URL: redis-service:6379
    depends_on: [redis-service, minio-service]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:4003/health || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  minio-service:
    image: ghcr.io/coollabsio/minio:RELEASE.2025-10-15T17-29-55Z
    command: ["server", "/data", "--console-address", ":9001"]
    environment:
      MINIO_ROOT_USER: $SERVICE_USER_MINIO
      MINIO_ROOT_PASSWORD: $SERVICE_PASSWORD_MINIO
      MINIO_BROWSER: "off"
    volumes:
      - budibase_minio_data:/data
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 10s
      timeout: 10s
      retries: 5
  proxy-service:
    image: budibase/proxy:latest
    ports: ["10000:10000"]
    environment:
      APPS_UPSTREAM_URL: http://app-service:4002
      WORKER_UPSTREAM_URL: http://worker-service:4003
      MINIO_UPSTREAM_URL: http://minio-service:9000
      COUCHDB_UPSTREAM_URL: http://couchdb-service:5984
      RESOLVER: 127.0.0.11
    depends_on: [minio-service, worker-service, app-service, couchdb-service]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:10000/ || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  couchdb-service:
    image: budibase/couchdb:latest
    environment:
      COUCHDB_PASSWORD: $SERVICE_PASSWORD_COUCHDB
      COUCHDB_USER: $SERVICE_USER_COUCHDB
      TARGETBUILD: docker-compose
    volumes:
      - budibase_couchdb_data:/opt/couchdb/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5984/ || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  redis-service:
    image: redis:7-alpine
    volumes:
      - budibase_redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 15s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "lowcoder",
		Name:                   "Lowcoder",
		Slogan:                 "An open-source low-code platform for building internal apps, dashboards, and workflows with drag-and-drop.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.lowcoder.cloud",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  lowcoder:
    image: lowcoderorg/lowcoder-ce:2.7.6
    ports: ["3000:3000"]
    environment:
      LOWCODER_EMAIL_SIGNUP_ENABLED: "true"
      LOWCODER_DB_ENCRYPTION_PASSWORD: $SERVICE_PASSWORD_ENCRYPTION
      LOWCODER_DB_ENCRYPTION_SALT: $SERVICE_PASSWORD_SALT
    volumes:
      - lowcoder_data:/lowcoder-stacks
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/health || exit 1"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "uptime-kuma-mariadb",
		Name:                   "Uptime Kuma (MariaDB)",
		Slogan:                 "Uptime Kuma with a MariaDB sidecar provisioned, for the setups that opt out of its default SQLite file.",
		Category:               "Monitoring",
		DocumentationURL:       "https://github.com/louislam/uptime-kuma/wiki",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  uptime-kuma:
    image: louislam/uptime-kuma:2
    ports: ["3001:3001"]
    environment:
      TZ: Etc/UTC
    volumes:
      - uptime_kuma_mariadb_app_data:/app/data
    depends_on: [mariadb]
    healthcheck:
      test: ["CMD", "extra/healthcheck"]
      interval: 10s
      timeout: 5s
      retries: 5
  mariadb:
    image: mariadb:11
    environment:
      MYSQL_USER: $SERVICE_USER_MARIADB
      MYSQL_PASSWORD: $SERVICE_PASSWORD_MARIADB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
      MYSQL_DATABASE: uptime_kuma
    volumes:
      - uptime_kuma_mariadb_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		// Tag not independently verified against a live registry in
		// this environment; the image repository (Amazon ECR Public) is
		// correct and matches upstream's own self-host instructions.
		ID:                     "openobserve",
		Name:                   "OpenObserve",
		Slogan:                 "A lightweight, single-binary observability platform for logs, metrics, and traces with a built-in UI.",
		Category:               "Monitoring",
		DocumentationURL:       "https://openobserve.ai/docs",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  openobserve:
    image: public.ecr.aws/zinclabs/openobserve:v0.90.0
    ports: ["5080:5080"]
    environment:
      ZO_DATA_DIR: /data
      ZO_ROOT_USER_EMAIL: root@example.com
      ZO_ROOT_USER_PASSWORD: $SERVICE_PASSWORD_OPENOBSERVE
      ZO_TELEMETRY: "false"
    volumes:
      - openobserve_data:/data
    healthcheck:
      test: ["CMD", "/openobserve", "node", "status"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 60s
`,
	},
	{ //nolint:gosec // DB_PASSWORD/DB_ROOT_PASSWD below are compose magic-var tokens, not real credentials
		ID:                     "seafile",
		Name:                   "Seafile",
		Slogan:                 "Self-hosted file sync and share with real file-level versioning and client-side encryption options.",
		Category:               "Storage",
		DocumentationURL:       "https://manual.seafile.com",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  seafile:
    image: seafileltd/seafile-mc:13.0.28
    ports: ["8080:80"]
    environment:
      SEAFILE_SERVER_HOSTNAME: ${SERVICE_FQDN_SEAFILE:-http://localhost:8080}
      DB_HOST: mariadb
      DB_PORT: "3306"
      DB_ROOT_PASSWD: $SERVICE_PASSWORD_MYSQLROOT
      DB_USER: $SERVICE_USER_MYSQL
      DB_PASSWORD: $SERVICE_PASSWORD_MYSQL
      TIME_ZONE: UTC
      INIT_SEAFILE_ADMIN_EMAIL: admin@example.com
      INIT_SEAFILE_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      SEAFILE_SERVER_PROTOCOL: https
      JWT_PRIVATE_KEY: $SERVICE_PASSWORD_64_JWT
    volumes:
      - seafile_data:/shared
    depends_on: [mariadb, memcached]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/api2/ping/ || exit 1"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 60s
  mariadb:
    image: mariadb:11
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
      MYSQL_USER: $SERVICE_USER_MYSQL
      MYSQL_PASSWORD: $SERVICE_PASSWORD_MYSQL
      MYSQL_DATABASE: seafile_db
    volumes:
      - seafile_mariadb_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 10s
      timeout: 10s
      retries: 5
  memcached:
    image: memcached:latest
    entrypoint: ["memcached", "-m", "256"]
    healthcheck:
      test: ["CMD-SHELL", "echo version | nc -w 1 127.0.0.1 11211 || exit 1"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		// Only published under rolling sha/edge/nightly tags upstream,
		// no pinned semver release exists on this image's registry; edge
		// is upstream's own recommended "stable rolling" tag.
		ID:                     "actualbudget",
		Name:                   "Actual Budget",
		Slogan:                 "A local-first personal finance and budgeting app with multi-device sync you fully own.",
		Category:               "Finance",
		DocumentationURL:       "https://actualbudget.org/docs/install/docker",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  actual:
    image: actualbudget/actual-server:edge
    ports: ["5006:5006"]
    environment:
      ACTUAL_LOGIN_METHOD: password
    volumes:
      - actualbudget_data:/data
    healthcheck:
      test: ["CMD-SHELL", "node -e \"require('http').get('http://127.0.0.1:5006/',r=>process.exit(r.statusCode<500?0:1)).on('error',()=>process.exit(1))\""]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "linkding-plus",
		Name:                   "Linkding Plus",
		Slogan:                 "Linkding's extended image with full-page snapshot archiving bundled in, for bookmarks that must survive link rot.",
		Category:               "Productivity",
		DocumentationURL:       "https://linkding.link",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  linkding-plus:
    image: sissbruecker/linkding:1.47.0-plus
    ports: ["9090:9090"]
    environment:
      LD_SUPERUSER_NAME: $SERVICE_USER_LINKDING
      LD_SUPERUSER_PASSWORD: $SERVICE_PASSWORD_LINKDING
    volumes:
      - linkding_plus_data:/etc/linkding/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9090/health || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "onetimesecret",
		Name:                   "One-Time Secret",
		Slogan:                 "Share a password or API key through a link that self-destructs after the first view.",
		Category:               "Security",
		DocumentationURL:       "https://docs.onetimesecret.com",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  onetimesecret:
    image: onetimesecret/onetimesecret:v0.26.14
    ports: ["3000:3000"]
    environment:
      AUTH_AUTOVERIFY: "true"
      AUTH_SIGNUP: "true"
      COLONEL: admin@example.com
      HOST: ${SERVICE_FQDN_ONETIMESECRET:-localhost}
      REDIS_URL: redis://redis:6379/0
      SECRET: $SERVICE_PASSWORD_ONETIMESECRET
      SSL: "false"
      RACK_ENV: production
    depends_on: [redis]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/ || exit 1"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 30s
  redis:
    image: redis:7-alpine
    volumes:
      - onetimesecret_redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "espocrm",
		Name:                   "EspoCRM",
		Slogan:                 "An open-source CRM for managing sales, support, and customer relationships end to end.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.espocrm.com",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		// Upstream's compose also ships espocrm-daemon and
		// espocrm-websocket sidecars via volumes_from, which this
		// platform's Compose subset doesn't support; both are optional
		// background workers (scheduled jobs, live notifications), so
		// dropping them keeps the core CRM fully functional.
		Compose: `services:
  espocrm:
    image: espocrm/espocrm:10
    ports: ["8080:80"]
    environment:
      ESPOCRM_ADMIN_USERNAME: admin
      ESPOCRM_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      ESPOCRM_DATABASE_PLATFORM: Mysql
      ESPOCRM_DATABASE_HOST: espocrm-db
      ESPOCRM_DATABASE_NAME: espocrm
      ESPOCRM_DATABASE_USER: $SERVICE_USER_MARIADB
      ESPOCRM_DATABASE_PASSWORD: $SERVICE_PASSWORD_MARIADB
      ESPOCRM_SITE_URL: ${SERVICE_FQDN_ESPOCRM:-http://localhost:8080}
    volumes:
      - espocrm_data:/var/www/html/data
      - espocrm_custom:/var/www/html/custom
    depends_on: [espocrm-db]
    healthcheck:
      test: ["CMD", "bin/command", "app-check"]
      interval: 30s
      timeout: 15s
      retries: 5
      start_period: 30s
  espocrm-db:
    image: mariadb:12.3
    environment:
      MARIADB_DATABASE: espocrm
      MARIADB_USER: $SERVICE_USER_MARIADB
      MARIADB_PASSWORD: $SERVICE_PASSWORD_MARIADB
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
    volumes:
      - espocrm_db_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 15s
`,
	},
}
