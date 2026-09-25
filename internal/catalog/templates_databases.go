package catalog

var databasesTemplates = []Template{
	{
		ID:                     "nocodb",
		Name:                   "NocoDB",
		Slogan:                 "Turn any database into a smart spreadsheet, with a real-time collaborative grid UI.",
		Category:               "Database Tools",
		DocumentationURL:       "https://docs.nocodb.com",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  nocodb:
    image: nocodb/nocodb:0.263.5
    ports: ["8080:8080"]
    volumes:
      - nocodb_data:/usr/app/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "pgadmin",
		Name:                   "pgAdmin",
		Slogan:                 "A full-featured web GUI for administering and querying PostgreSQL databases.",
		Category:               "Database Tools",
		DocumentationURL:       "https://www.pgadmin.org/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  pgadmin:
    image: dpage/pgadmin4:8.14
    ports: ["8080:80"]
    environment:
      PGADMIN_DEFAULT_EMAIL: admin@example.com
      PGADMIN_DEFAULT_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - pgadmin_data:/var/lib/pgadmin
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/misc/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 90s
`,
	},
	{
		ID:                     "redisinsight",
		Name:                   "RedisInsight",
		Slogan:                 "A GUI for browsing keys, running commands, and profiling performance on any Redis instance.",
		Category:               "Database Tools",
		DocumentationURL:       "https://redis.io/docs/latest/operate/redisinsight/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  redisinsight:
    image: redis/redisinsight:2.70
    ports: ["5540:5540"]
    environment:
      RI_APP_HOST: "0.0.0.0"
      RI_APP_PORT: "5540"
      RI_ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
    volumes:
      - redisinsight_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5540/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "qdrant",
		Name:                   "Qdrant",
		Slogan:                 "A vector similarity search engine for storing, searching, and managing embeddings.",
		Category:               "Database Tools",
		DocumentationURL:       "https://qdrant.tech/documentation/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  qdrant:
    image: qdrant/qdrant:v1.12.4
    ports: ["6333:6333"]
    environment:
      QDRANT__SERVICE__API_KEY: $SERVICE_HEX_64_APIKEY
    volumes:
      - qdrant_data:/qdrant/storage
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:6333/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "influxdb",
		Name:                   "InfluxDB",
		Slogan:                 "An open-source time-series database for metrics, events, and IoT analytics.",
		Category:               "Database Tools",
		DocumentationURL:       "https://docs.influxdata.com/influxdb/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  influxdb:
    image: influxdb:2.7-alpine
    ports: ["8086:8086"]
    environment:
      INFLUXDB_INIT_MODE: setup
      INFLUXDB_INIT_USERNAME: $SERVICE_USER_ADMIN
      INFLUXDB_INIT_PASSWORD: $SERVICE_PASSWORD_ADMIN
      INFLUXDB_INIT_ORG: main
      INFLUXDB_INIT_BUCKET: main
      INFLUXDB_INIT_ADMIN_TOKEN: $SERVICE_HEX_64_ADMINTOKEN
    volumes:
      - influxdb_data:/var/lib/influxdb2
      - influxdb_config:/etc/influxdb2
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8086/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "baserow",
		Name:                   "Baserow",
		Slogan:                 "A no-code database and spreadsheet hybrid you can build internal tools and apps on top of.",
		Category:               "Database Tools",
		DocumentationURL:       "https://baserow.io/docs/installation%2Finstall-with-docker",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  baserow:
    image: baserow/baserow:2.3.3
    ports: ["80:80"]
    environment:
      BASEROW_PUBLIC_URL: ${SERVICE_FQDN_BASEROW:-http://localhost}
    volumes:
      - baserow_data:/baserow/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/api/_health/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 240s
`,
	},
	{
		ID:                     "clickhouse",
		Name:                   "ClickHouse",
		Slogan:                 "A columnar database built for fast analytical queries over large datasets.",
		Category:               "Database Tools",
		DocumentationURL:       "https://hub.docker.com/r/clickhouse/clickhouse-server",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  clickhouse:
    image: clickhouse/clickhouse-server:26.3-alpine
    ports: ["8123:8123"]
    environment:
      CLICKHOUSE_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - clickhouse_data:/var/lib/clickhouse
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8123/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "adminer",
		Name:                   "Adminer",
		Slogan:                 "A single-file database admin tool for MySQL, PostgreSQL, SQLite, and more.",
		Category:               "Database Tools",
		DocumentationURL:       "https://www.adminer.org",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  adminer:
    image: adminer:5
    ports: ["8080:8080"]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "bytebase",
		Name:                   "Bytebase",
		Slogan:                 "A database schema change and migration tool with review workflows built in.",
		Category:               "Database Tools",
		DocumentationURL:       "https://docs.bytebase.com/get-started/step-by-step/deploy-with-docker",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  bytebase:
    image: bytebase/bytebase:3.9.2
    command: ["--data", "/var/opt/bytebase", "--port", "8080"]
    ports: ["8080:8080"]
    volumes:
      - bytebase_data:/var/opt/bytebase
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "cloudbeaver",
		Name:                   "CloudBeaver",
		Slogan:                 "A web-based database manager for browsing and querying Postgres, MySQL, SQLite and more.",
		Category:               "Database Tools",
		DocumentationURL:       "https://dbeaver.com/docs/cloudbeaver/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  cloudbeaver:
    image: dbeaver/cloudbeaver:24.1.0
    ports: ["8978:8978"]
    volumes:
      - cloudbeaver_workspace:/opt/cloudbeaver/workspace
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8978/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
}
