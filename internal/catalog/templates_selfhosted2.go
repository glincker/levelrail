package catalog

var selfhosted2Templates = []Template{
	{
		ID:                     "drupal",
		Name:                   "Drupal",
		Slogan:                 "A mature, flexible CMS for structured content, multilingual sites, and large editorial teams.",
		Category:               "Applications",
		DocumentationURL:       "https://www.drupal.org/docs",
		RecommendedMemoryBytes: 1073741824,
		// Database credentials are entered in the Drupal installer: host db, user and database drupal, password from the db service env.
		Compose: `services:
  drupal:
    image: drupal:11-apache
    ports: ["8080:80"]
    volumes:
      - drupal_modules:/var/www/html/modules
      - drupal_profiles:/var/www/html/profiles
      - drupal_themes:/var/www/html/themes
      - drupal_sites:/var/www/html/sites
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: drupal
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: drupal
    volumes:
      - drupal_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U drupal -d drupal"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "joomla",
		Name:                   "Joomla",
		Slogan:                 "A long-established CMS with a large extension ecosystem for business sites, portals, and communities.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.joomla.org",
		RecommendedMemoryBytes: 805306368,
		Compose: `services:
  joomla:
    image: joomla:5-apache
    ports: ["8080:80"]
    environment:
      JOOMLA_DB_HOST: db
      JOOMLA_DB_USER: joomla
      JOOMLA_DB_PASSWORD: $SERVICE_PASSWORD_DB
      JOOMLA_DB_NAME: joomla
    volumes:
      - joomla_data:/var/www/html
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
      MARIADB_USER: joomla
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
      MARIADB_DATABASE: joomla
    volumes:
      - joomla_db_data:/var/lib/mysql
`,
	},
	{
		ID:                     "mediawiki",
		Name:                   "MediaWiki",
		Slogan:                 "The wiki engine behind Wikipedia, for large collaborative knowledge bases with rich version history.",
		Category:               "Applications",
		DocumentationURL:       "https://www.mediawiki.org/wiki/Manual:Contents",
		RecommendedMemoryBytes: 536870912,
		// Database credentials are entered in the MediaWiki installer: host db, user wikiuser, database mediawiki.
		Compose: `services:
  mediawiki:
    image: mediawiki:1.43
    ports: ["8080:80"]
    volumes:
      - mediawiki_images:/var/www/html/images
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: wikiuser
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: mediawiki
    volumes:
      - mediawiki_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U wikiuser -d mediawiki"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "odoo",
		Name:                   "Odoo",
		Slogan:                 "An all-in-one business suite covering CRM, sales, inventory, accounting, and a website builder.",
		Category:               "Applications",
		DocumentationURL:       "https://www.odoo.com/documentation",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  odoo:
    image: odoo:18
    ports: ["8069:8069"]
    environment:
      HOST: db
      USER: odoo
      PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - odoo_web_data:/var/lib/odoo
      - odoo_addons:/mnt/extra-addons
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8069/web/health || wget -q -O /dev/null http://127.0.0.1:8069/web/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 90s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: odoo
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: postgres
    volumes:
      - odoo_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U odoo -d postgres"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "dolibarr",
		Name:                   "Dolibarr",
		Slogan:                 "An open-source ERP and CRM for small businesses: invoicing, contacts, stock, and accounting in one app.",
		Category:               "Finance",
		DocumentationURL:       "https://wiki.dolibarr.org",
		RecommendedMemoryBytes: 805306368,
		Compose: `services:
  dolibarr:
    image: dolibarr/dolibarr:23.0.4
    ports: ["8080:80"]
    environment:
      DOLI_DB_HOST: db
      DOLI_DB_NAME: dolibarr
      DOLI_DB_USER: dolibarr
      DOLI_DB_PASSWORD: $SERVICE_PASSWORD_DB
      DOLI_ADMIN_LOGIN: admin
      DOLI_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      DOLI_URL_ROOT: ${SERVICE_FQDN_DOLIBARR:-http://localhost:8080}
    volumes:
      - dolibarr_documents:/var/www/documents
      - dolibarr_custom:/var/www/html/custom
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 90s
  db:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
      MARIADB_USER: dolibarr
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
      MARIADB_DATABASE: dolibarr
    volumes:
      - dolibarr_db_data:/var/lib/mysql
`,
	},
	{
		ID:                     "temporal",
		Name:                   "Temporal",
		Slogan:                 "Durable workflow execution: write long-running business logic as code and let Temporal survive failures.",
		Category:               "Automation",
		DocumentationURL:       "https://docs.temporal.io",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  temporal:
    image: temporalio/auto-setup:1.29.7
    ports: ["7233:7233"]
    environment:
      DB: postgres12
      DB_PORT: "5432"
      POSTGRES_USER: temporal
      POSTGRES_PWD: $SERVICE_PASSWORD_DB
      POSTGRES_SEEDS: db
    healthcheck:
      test: ["CMD-SHELL", "tctl --address 127.0.0.1:7233 cluster health || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 60s
  temporal-ui:
    image: temporalio/ui:2.54.1
    ports: ["8080:8080"]
    environment:
      TEMPORAL_ADDRESS: temporal:7233
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: temporal
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: temporal
    volumes:
      - temporal_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U temporal -d temporal"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{ //nolint:gosec // credential in URL below is a compose magic-var token, not a real credential
		ID:                     "prefect",
		Name:                   "Prefect",
		Slogan:                 "Python-native workflow orchestration with scheduling, retries, and a dashboard for every flow run.",
		Category:               "Automation",
		DocumentationURL:       "https://docs.prefect.io",
		RecommendedMemoryBytes: 1073741824,
		// Rolling 3-latest tag: Prefect publishes exact versions with a python suffix.
		Compose: `services:
  prefect:
    image: prefecthq/prefect:3-latest
    command: ["prefect", "server", "start", "--host", "0.0.0.0"]
    ports: ["4200:4200"]
    environment:
      PREFECT_HOME: /data
      PREFECT_API_DATABASE_CONNECTION_URL: postgresql+asyncpg://prefect:$SERVICE_PASSWORD_DB@db:5432/prefect
    volumes:
      - prefect_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:4200/api/health || wget -q -O /dev/null http://127.0.0.1:4200/api/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: prefect
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: prefect
    volumes:
      - prefect_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U prefect -d prefect"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "windmill",
		Name:                   "Windmill",
		Slogan:                 "Turn scripts in Python, TypeScript, Go, and SQL into webhooks, workflows, and internal UIs.",
		Category:               "Automation",
		DocumentationURL:       "https://www.windmill.dev/docs",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  windmill:
    image: ghcr.io/windmill-labs/windmill:1.365.0
    ports: ["8000:8000"]
    environment:
      MODE: standalone
      DATABASE_URL: postgres://windmill:$SERVICE_PASSWORD_DB@db:5432/windmill?sslmode=disable
      BASE_URL: ${SERVICE_FQDN_WINDMILL:-http://localhost:8000}
    volumes:
      - windmill_worker_logs:/tmp/windmill/logs
      - windmill_worker_cache:/tmp/windmill/cache
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/api/version || wget -q -O /dev/null http://127.0.0.1:8000/api/version || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: windmill
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: windmill
    volumes:
      - windmill_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U windmill -d windmill"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "jellyseerr",
		Name:                   "Jellyseerr",
		Slogan:                 "A request manager for Jellyfin, Plex, and Emby libraries, wired to Sonarr and Radarr.",
		Category:               "Media",
		DocumentationURL:       "https://docs.jellyseerr.dev",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  jellyseerr:
    image: fallenbagel/jellyseerr:2.7.3
    ports: ["5055:5055"]
    volumes:
      - jellyseerr_config:/app/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5055/api/v1/status || wget -q -O /dev/null http://127.0.0.1:5055/api/v1/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "owncast",
		Name:                   "Owncast",
		Slogan:                 "Run your own live video streaming server with built-in chat, no third-party platform required.",
		Category:               "Media",
		DocumentationURL:       "https://owncast.online/docs",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  owncast:
    image: owncast/owncast:0.3.0
    ports: ["8080:8080", "1935:1935"]
    volumes:
      - owncast_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/api/status || wget -q -O /dev/null http://127.0.0.1:8080/api/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "pinchflat",
		Name:                   "Pinchflat",
		Slogan:                 "Automatically download and organize YouTube channels and playlists into your media library.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/kieraneglin/pinchflat/wiki",
		RecommendedMemoryBytes: 536870912,
		// Rolling tag: upstream publishes only latest and nightly.
		Compose: `services:
  pinchflat:
    image: ghcr.io/kieraneglin/pinchflat:latest
    ports: ["8945:8945"]
    volumes:
      - pinchflat_config:/config
      - pinchflat_downloads:/downloads
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8945/healthcheck || wget -q -O /dev/null http://127.0.0.1:8945/healthcheck || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "metube",
		Name:                   "MeTube",
		Slogan:                 "A tidy web front end for yt-dlp: paste a link, pick a format, and download video or audio.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/alexta69/metube",
		RecommendedMemoryBytes: 268435456,
		// Rolling tag: upstream publishes only latest.
		Compose: `services:
  metube:
    image: ghcr.io/alexta69/metube:latest
    ports: ["8081:8081"]
    volumes:
      - metube_downloads:/downloads
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8081/ || wget -q -O /dev/null http://127.0.0.1:8081/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "apprise-api",
		Name:                   "Apprise API",
		Slogan:                 "A single HTTP endpoint that fans a notification out to 100+ services such as Slack, Discord, and email.",
		Category:               "Communication",
		DocumentationURL:       "https://github.com/caronc/apprise-api",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  apprise-api:
    image: caronc/apprise:1.5.4
    ports: ["8000:8000"]
    volumes:
      - apprise_config:/config
      - apprise_attach:/attachments
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/status || wget -q -O /dev/null http://127.0.0.1:8000/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
}
