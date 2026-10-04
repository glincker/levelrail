package catalog

// catalogBatch4Templates ports a fourth wave of well-known templates from
// Coolify's own public service catalog (ADR 015), each adapted to this
// platform's narrower Compose subset: no bind-mount config injection, no
// cap_add/sysctls, no privileged/network_mode, no UDP ports. Candidates
// that fundamentally need one of those (Dozzle's docker.sock mount,
// WireGuard Easy's NET_ADMIN capability, ESPHome's host networking) were
// left out rather than shipped half-working.
var catalogBatch4Templates = []Template{
	{
		// LinuxServer's own Plex tag tracks Plex's point releases closely;
		// this exact build string could not be verified against a live
		// registry in this environment.
		ID:                     "plex",
		Name:                   "Plex",
		Slogan:                 "A media server that organizes your movies, TV, and music and streams them to any device.",
		Category:               "Media",
		DocumentationURL:       "https://docs.linuxserver.io/images/docker-plex/",
		RecommendedMemoryBytes: 2147483648, // 2Gi
		Compose: `services:
  plex:
    image: lscr.io/linuxserver/plex:1.41.9.9937-c6926edb6-ls213
    ports: ["32400:32400"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
      VERSION: docker
    volumes:
      - plex_config:/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:32400/identity"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		// SYNAPSE_SERVER_NAME becomes the permanent domain in every user's
		// ID (@user:domain) and can't change after the first boot; the
		// official image auto-generates homeserver.yaml from these two
		// env vars when /data has none yet, no entrypoint override needed.
		ID:                     "matrix-synapse",
		Name:                   "Matrix Synapse",
		Slogan:                 "A federated, end-to-end encrypted chat server implementing the Matrix protocol.",
		Category:               "Communication",
		DocumentationURL:       "https://element-hq.github.io/synapse/latest/setup/installation.html",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  synapse:
    image: matrixdotorg/synapse:v1.145.0
    ports: ["8008:8008"]
    environment:
      SYNAPSE_SERVER_NAME: ${SERVICE_FQDN_SYNAPSE:-localhost}
      SYNAPSE_REPORT_STATS: "no"
    volumes:
      - synapse_data:/data
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:8008/health"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
`,
	},
	{
		ID:                     "phpmyadmin",
		Name:                   "phpMyAdmin",
		Slogan:                 "A web-based admin tool for MySQL and MariaDB, built for ad-hoc use against any reachable database.",
		Category:               "Database Tools",
		DocumentationURL:       "https://www.phpmyadmin.net/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// No bundled database: PMA_ARBITRARY lets the login screen accept
		// any host, so one instance can administer several databases.
		Compose: `services:
  phpmyadmin:
    image: phpmyadmin:5.2.2
    ports: ["8080:80"]
    environment:
      PMA_ARBITRARY: "1"
      UPLOAD_LIMIT: 64M
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:80"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "redis-insight",
		Name:                   "Redis Insight",
		Slogan:                 "A GUI for exploring keys, running commands, and profiling workloads on any Redis-compatible server.",
		Category:               "Database Tools",
		DocumentationURL:       "https://redis.io/docs/latest/operate/redisinsight/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  redisinsight:
    image: redis/redisinsight:2.70
    ports: ["5540:5540"]
    environment:
      RI_APP_HOST: 0.0.0.0
      RI_APP_PORT: "5540"
    volumes:
      - redisinsight_data:/data
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://127.0.0.1:5540/api/health"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 10s
`,
	},
	{
		// mosquitto-no-auth.conf ships inside the official image for
		// exactly this case (no bind mount available here): it sets
		// allow_anonymous, fine for a mesh-internal broker, not for one
		// exposed publicly without a reverse-proxy auth layer in front.
		ID:                     "mosquitto",
		Name:                   "Eclipse Mosquitto",
		Slogan:                 "A lightweight MQTT broker for connecting IoT devices, sensors, and home automation hubs.",
		Category:               "IoT",
		DocumentationURL:       "https://mosquitto.org/documentation/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  mosquitto:
    image: eclipse-mosquitto:2.0.20
    ports: ["1883:1883"]
    command: ["mosquitto", "-c", "/mosquitto-no-auth.conf"]
    volumes:
      - mosquitto_data:/mosquitto/data
    healthcheck:
      test: ["CMD-SHELL", "mosquitto_pub -h 127.0.0.1 -t healthcheck -m ping -q 0 || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
`,
	},
	{
		ID:                     "minecraft",
		Name:                   "Minecraft Server",
		Slogan:                 "A vanilla Minecraft Java server that downloads and runs the selected version on first boot.",
		Category:               "Applications",
		DocumentationURL:       "https://github.com/itzg/docker-minecraft-server",
		RecommendedMemoryBytes: 3221225472, // 3Gi
		Compose: `services:
  minecraft:
    image: itzg/minecraft-server:java21
    ports: ["25565:25565"]
    environment:
      EULA: "TRUE"
      TYPE: VANILLA
      MEMORY: 2G
      DIFFICULTY: normal
      MAX_PLAYERS: "10"
    volumes:
      - minecraft_data:/data
    healthcheck:
      test: ["CMD-SHELL", "mc-health"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 60s
`,
	},
	{
		// Tag not independently verified against a live registry in this
		// environment; the image repository matches upstream's own
		// self-host instructions.
		ID:                     "whoogle",
		Name:                   "Whoogle Search",
		Slogan:                 "A privacy-focused front end for Google search results, with no tracking, ads, or JavaScript required.",
		Category:               "Security",
		DocumentationURL:       "https://github.com/benbusby/whoogle-search",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  whoogle:
    image: benbusby/whoogle-search:0.9.3
    ports: ["5000:5000"]
    environment:
      WHOOGLE_CONFIG_COUNTRY: US
      WHOOGLE_CONFIG_LANGUAGE: lang_en
    volumes:
      - whoogle_data:/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:5000"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		// LinuxServer tag not independently verified against a live
		// registry in this environment.
		ID:                     "pairdrop",
		Name:                   "PairDrop",
		Slogan:                 "A self-hosted AirDrop-style app for sending files and messages between devices on the same network.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/schlagmichdoch/PairDrop",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  pairdrop:
    image: lscr.io/linuxserver/pairdrop:1.19.0-ls94
    ports: ["3000:3000"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:3000"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		ID:                     "checkmate",
		Name:                   "Checkmate",
		Slogan:                 "An open-source uptime and server monitoring app with incident history and status pages.",
		Category:               "Monitoring",
		DocumentationURL:       "https://docs.checkmate.so",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  checkmate:
    image: ghcr.io/bluewave-labs/checkmate-backend-mono-multiarch:v3.2.0
    ports: ["52345:52345"]
    environment:
      UPTIME_APP_API_BASE_URL: ${SERVICE_FQDN_CHECKMATE:-http://localhost:52345}/api/v1
      UPTIME_APP_CLIENT_HOST: ${SERVICE_FQDN_CHECKMATE:-http://localhost:52345}
      DB_CONNECTION_STRING: mongodb://mongo:27017/checkmate
      JWT_SECRET: $SERVICE_PASSWORD_64_JWT
    depends_on: [mongo]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:52345/ || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  mongo:
    image: mongo:7
    volumes:
      - checkmate_mongo_data:/data/db
`,
	},
	{
		ID:                     "traccar",
		Name:                   "Traccar",
		Slogan:                 "An open-source GPS tracking platform supporting over 170 device protocols.",
		Category:               "IoT",
		DocumentationURL:       "https://www.traccar.org/documentation/",
		RecommendedMemoryBytes: 1073741824, // 1Gi
		Compose: `services:
  traccar:
    image: traccar/traccar:6.6
    ports: ["8082:8082"]
    environment:
      CONFIG_USE_ENVIRONMENT_VARIABLES: "true"
      DATABASE_DRIVER: org.postgresql.Driver
      DATABASE_URL: jdbc:postgresql://traccar-db:5432/traccar
      DATABASE_USER: traccar
      DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
    depends_on: [traccar-db]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8082 || exit 1"]
      interval: 15s
      timeout: 10s
      retries: 5
      start_period: 30s
  traccar-db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: traccar
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: traccar
    volumes:
      - traccar_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U traccar -d traccar"]
      interval: 5s
      timeout: 5s
      retries: 5
`,
	},
	{
		// cockpithq only publishes rolling "core-latest"/"latest" tags, no
		// pinned semver release exists on this image's registry.
		ID:                     "cockpit-cms",
		Name:                   "Cockpit CMS",
		Slogan:                 "A headless content platform for managing structured content behind a clean API, without a big CMS footprint.",
		Category:               "Applications",
		DocumentationURL:       "https://getcockpit.com/documentation/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  cockpit-cms:
    image: cockpithq/cockpit:core-latest
    ports: ["80:80"]
    volumes:
      - cockpitcms_config:/var/www/html/config
      - cockpitcms_storage:/var/www/html/storage
      - cockpitcms_spaces:/var/www/html/.spaces
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://127.0.0.1/api/system/healthcheck"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 15s
`,
	},
	{
		// No basic-auth htpasswd configured (upstream's own setup needs a
		// bind-mounted entrypoint script, unsupported here): fine for a
		// mesh-internal registry, put a real auth layer in front before
		// exposing it publicly.
		ID:                     "docker-registry",
		Name:                   "Docker Registry",
		Slogan:                 "A private registry for storing and distributing your own container images.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://distribution.github.io/distribution/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  registry:
    image: registry:3
    ports: ["5000:5000"]
    environment:
      REGISTRY_STORAGE_DELETE_ENABLED: "true"
    volumes:
      - registry_data:/var/lib/registry
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://127.0.0.1:5000/v2/"]
      interval: 10s
      timeout: 10s
      retries: 5
`,
	},
	{
		// Tag not independently verified against a live registry in this
		// environment; the image repository matches upstream's own docs.
		ID:                     "flipt",
		Name:                   "Flipt",
		Slogan:                 "A self-hosted feature flag and experimentation platform with a built-in UI and REST/gRPC APIs.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://www.flipt.io/docs",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  flipt:
    image: docker.flipt.io/flipt/flipt:v1.60.0
    ports: ["8080:8080"]
    volumes:
      - flipt_data:/var/opt/flipt
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://127.0.0.1:8080/health"]
      interval: 10s
      timeout: 10s
      retries: 5
      start_period: 10s
`,
	},
}
