package catalog

var monitoringTemplates = []Template{
	{
		ID:                     "uptime-kuma",
		Name:                   "Uptime Kuma",
		Slogan:                 "A self-hosted uptime monitor with a clean dashboard for HTTP, TCP, DNS, and ping checks.",
		Category:               "Monitoring",
		DocumentationURL:       "https://github.com/louislam/uptime-kuma/wiki",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  uptime-kuma:
    image: louislam/uptime-kuma:1.23.13
    ports: ["3001:3001"]
    volumes:
      - uptime_kuma_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3001/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "grafana",
		Name:                   "Grafana",
		Slogan:                 "Dashboards and exploration for metrics, logs, and traces from any data source.",
		Category:               "Monitoring",
		DocumentationURL:       "https://grafana.com/docs/grafana/latest/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  grafana:
    image: grafana/grafana:11.2.0
    ports: ["3000:3000"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - grafana_data:/var/lib/grafana
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "prometheus",
		Name:                   "Prometheus",
		Slogan:                 "A metrics time-series database and alerting engine built for pull-based scraping.",
		Category:               "Monitoring",
		DocumentationURL:       "https://prometheus.io/docs/introduction/overview/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  prometheus:
    image: prom/prometheus:v2.54.1
    ports: ["9090:9090"]
    volumes:
      - prometheus_data:/prometheus
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:9090/-/healthy || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "changedetection",
		Name:                   "Changedetection.io",
		Slogan:                 "Monitor any webpage for changes and get notified the moment content updates.",
		Category:               "Monitoring",
		DocumentationURL:       "https://github.com/dgtlmoon/changedetection.io/wiki",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  changedetection:
    image: ghcr.io/dgtlmoon/changedetection.io:0.49.12
    ports: ["5000:5000"]
    volumes:
      - changedetection_data:/datastore
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5000/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "healthchecks",
		Name:                   "Healthchecks",
		Slogan:                 "Cron job and scheduled task monitoring: get alerted the moment a periodic job stops checking in.",
		Category:               "Monitoring",
		DocumentationURL:       "https://healthchecks.io/docs/self_hosted/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  healthchecks:
    image: healthchecks/healthchecks:v4.2
    ports: ["8000:8000"]
    environment:
      DB: postgres
      DB_HOST: db
      DB_NAME: healthchecks
      DB_USER: healthchecks
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      ALLOWED_HOSTS: "*"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: healthchecks
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: healthchecks
    volumes:
      - healthchecks_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "diun",
		Name:                   "Diun",
		Slogan:                 "Watches your running containers and notifies you the moment a new image tag is published.",
		Category:               "Monitoring",
		DocumentationURL:       "https://crazymax.dev/diun/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment. Diun's Docker provider normally watches
		// events via a bind-mounted /var/run/docker.sock, which this
		// platform's compose subset can't express yet (named volumes
		// only, no bind mounts), so that provider stays inactive until
		// bind-mount support lands.
		Compose: `services:
  diun:
    image: crazymax/diun:4.29.0
    environment:
      TZ: "Etc/UTC"
      LOG_LEVEL: "info"
      DIUN_WATCH_WORKERS: "20"
      DIUN_WATCH_SCHEDULE: "0 */6 * * *"
      DIUN_PROVIDERS_DOCKER: "true"
      DIUN_PROVIDERS_DOCKER_WATCHBYDEFAULT: "true"
    volumes:
      - diun_data:/data
    healthcheck:
      test: ["CMD-SHELL", "pgrep diun || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "librespeed",
		Name:                   "LibreSpeed",
		Slogan:                 "A lightweight, self-hosted internet speed test with no ads, tracking, or Flash required.",
		Category:               "Monitoring",
		DocumentationURL:       "https://github.com/librespeed/speedtest",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  librespeed:
    image: ghcr.io/librespeed/speedtest:5.4.5
    ports: ["82:82"]
    environment:
      MODE: standalone
      TELEMETRY: "false"
      WEBPORT: "82"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:82/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "glances",
		Name:                   "Glances",
		Slogan:                 "A cross-platform system monitor showing CPU, memory, disk, and network at a glance.",
		Category:               "Monitoring",
		DocumentationURL:       "https://nicolargo.github.io/glances/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Real Glances setups bind-mount the host Docker socket to also
		// show per-container stats; this platform's compose subset only
		// supports named volumes (no bind mounts), so only host-level
		// CPU/memory/disk/network is wired up here. Only published under
		// a rolling :latest tag upstream; this pinned version couldn't be
		// verified against a live registry in this environment.
		Compose: `services:
  glances:
    image: nicolargo/glances:3.4.0-full
    ports: ["61208:61208"]
    environment:
      GLANCES_OPT: "-w"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:61208/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "statusnook",
		Name:                   "Statusnook",
		Slogan:                 "Deploy a status page and start monitoring endpoints in minutes.",
		Category:               "Monitoring",
		DocumentationURL:       "https://statusnook.com",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  statusnook:
    image: goksan/statusnook:1.4.0
    ports: ["8000:8000"]
    volumes:
      - statusnook_data:/app/statusnook-data
    healthcheck:
      test: ["CMD-SHELL", "sh -c ': < /dev/tcp/127.0.0.1/8000' || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "beszel",
		Name:                   "Beszel",
		Slogan:                 "A lightweight server monitoring hub with historical stats for CPU, memory, disk, and network.",
		Category:               "Monitoring",
		DocumentationURL:       "https://beszel.dev/guide/getting-started",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  beszel:
    image: henrygd/beszel:0.19.0
    ports: ["8090:8090"]
    environment:
      APP_URL: ${SERVICE_FQDN_BESZEL:-http://localhost:8090}
    volumes:
      - beszel_data:/beszel_data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8090/api/health"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // MATOMO_DATABASE_PASSWORD below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "matomo",
		Name:                   "Matomo",
		Slogan:                 "A privacy-friendly, self-hosted alternative to Google Analytics with full data ownership.",
		Category:               "Analytics",
		DocumentationURL:       "https://matomo.org/faq/how-to-install/install-matomo-with-docker/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  matomo:
    image: matomo:5.13.0-apache
    ports: ["8080:80"]
    environment:
      MATOMO_DATABASE_HOST: db
      MATOMO_DATABASE_USERNAME: matomo
      MATOMO_DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
      MATOMO_DATABASE_DBNAME: matomo
    volumes:
      - matomo_data:/var/www/html
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: mariadb:10.11
    environment:
      MYSQL_DATABASE: matomo
      MYSQL_USER: matomo
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - matomo_db_data:/var/lib/mysql
`,
	},
	{
		ID:                     "tautulli",
		Name:                   "Tautulli",
		Slogan:                 "Monitors your Plex server and shows who watched what, with history, stats, and notifications.",
		Category:               "Monitoring",
		DocumentationURL:       "https://docs.tautulli.com",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  tautulli:
    image: lscr.io/linuxserver/tautulli:2.15.0
    ports: ["8181:8181"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - tautulli_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8181/status"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
}
