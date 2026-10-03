package catalog

// catalogBatch2Templates is a third wave of the Coolify-catalog import
// (ADR 015), picked from genuine gaps against the 265+ templates already
// shipped. Dropped: supabase and apache-superset (both need a
// bind-mounted config file this platform's no-bind-mount compose subset
// can't inject), dozzle and github-runner (need /var/run/docker.sock,
// which internal/bindmount forbids), wireguard-easy and esphome (need
// cap_add/sysctls or network_mode: host, none of which this compose
// subset models).
var catalogBatch2Templates = []Template{
	{
		ID:                     "phpmyadmin",
		Name:                   "phpMyAdmin",
		Slogan:                 "A web-based admin UI for MySQL and MariaDB: browse, query, import, and export databases.",
		Category:               "Database Tools",
		DocumentationURL:       "https://docs.phpmyadmin.net/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// platform has no fixed host to pre-fill, so PMA_ARBITRARY lets an
		// operator point it at any reachable MySQL/MariaDB server from the
		// login screen instead of baking one connection in.
		Compose: `services:
  phpmyadmin:
    image: lscr.io/linuxserver/phpmyadmin:latest
    ports: ["8080:80"]
    environment:
      PUID: "1000"
      PGID: "1000"
      PMA_ARBITRARY: "1"
    volumes:
      - phpmyadmin_config:/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:80"]
      interval: 10s
      timeout: 5s
      retries: 15
      start_period: 15s
`,
	},
	{ //nolint:gosec // MOSQUITTO_USERNAME/PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "mosquitto",
		Name:                   "Eclipse Mosquitto",
		Slogan:                 "An MQTT broker for IoT and home-automation messaging, with password auth generated at first boot.",
		Category:               "IoT",
		DocumentationURL:       "https://mosquitto.org/documentation/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Upstream's own template bind-mounts ./mosquitto/config and
		// writes mosquitto.conf there from a command: string; this uses a
		// named volume instead and generates the same conf and a
		// password file inside it at container start, the same pattern
		// templates_devtools_batch.go's docker-registry-auth entry uses
		// for its htpasswd file.
		Compose: `services:
  mosquitto:
    image: eclipse-mosquitto:2.0.18
    ports: ["1883:1883"]
    environment:
      MOSQUITTO_USERNAME: $SERVICE_USER_MOSQUITTO
      MOSQUITTO_PASSWORD: $SERVICE_PASSWORD_MOSQUITTO
    volumes:
      - mosquitto_config:/mosquitto/config
      - mosquitto_data:/mosquitto/data
    command: ["/bin/sh", "-c", "mkdir -p /mosquitto/config && printf 'listener 1883\nallow_anonymous false\npassword_file /mosquitto/config/passwords\n' > /mosquitto/config/mosquitto.conf && touch /mosquitto/config/passwords && mosquitto_passwd -b /mosquitto/config/passwords \"$MOSQUITTO_USERNAME\" \"$MOSQUITTO_PASSWORD\" && exec mosquitto -c /mosquitto/config/mosquitto.conf"]
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 1883 || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 10s
`,
	},
	{
		ID:                     "whoogle",
		Name:                   "Whoogle Search",
		Slogan:                 "A privacy-respecting search frontend for Google results, with no tracking, ads, or JavaScript required.",
		Category:               "Applications",
		DocumentationURL:       "https://github.com/benbusby/whoogle-search",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  whoogle:
    image: benbusby/whoogle-search:latest
    ports: ["5000:5000"]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:5000"]
      interval: 10s
      timeout: 5s
      retries: 15
      start_period: 15s
`,
	},
	{
		ID:                     "pairdrop",
		Name:                   "PairDrop",
		Slogan:                 "A browser-based, cross-platform AirDrop alternative for sending files between devices on the same network.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/schlagmichdoch/PairDrop",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  pairdrop:
    image: lscr.io/linuxserver/pairdrop:latest
    ports: ["3000:3000"]
    environment:
      PUID: "1000"
      PGID: "1000"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:3000"]
      interval: 10s
      timeout: 5s
      retries: 15
      start_period: 15s
`,
	},
	{
		ID:                     "babybuddy",
		Name:                   "Baby Buddy",
		Slogan:                 "Track sleep, feeding, diaper changes, and growth for a baby or toddler, with charts and timers.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.baby-buddy.net/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  babybuddy:
    image: lscr.io/linuxserver/babybuddy:latest
    ports: ["8000:8000"]
    environment:
      PUID: "1000"
      PGID: "1000"
      CSRF_TRUSTED_ORIGINS: ${SERVICE_FQDN_BABYBUDDY:-http://localhost:8000}
    volumes:
      - babybuddy_config:/config
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:8000"]
      interval: 10s
      timeout: 5s
      retries: 15
      start_period: 20s
`,
	},
	{ //nolint:gosec // MYSQL_PASSWORD/REDIS_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "castopod",
		Name:                   "Castopod",
		Slogan:                 "An open-source podcast hosting platform with built-in analytics, a web player, and ActivityPub federation.",
		Category:               "Media",
		DocumentationURL:       "https://docs.castopod.org/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  castopod:
    image: castopod/castopod:1.15.4
    ports: ["8080:8080"]
    environment:
      MYSQL_DATABASE: castopod
      MYSQL_USER: $SERVICE_USER_MYSQL
      MYSQL_PASSWORD: $SERVICE_PASSWORD_MYSQL
      CP_DISABLE_HTTPS: "1"
      CP_BASEURL: ${SERVICE_FQDN_CASTOPOD:-http://localhost:8080}
      CP_ANALYTICS_SALT: $SERVICE_REALBASE64_64_SALT
      CP_CACHE_HANDLER: redis
      CP_REDIS_HOST: redis
      CP_REDIS_PASSWORD: $SERVICE_PASSWORD_REDIS
    volumes:
      - castopod_media:/var/www/castopod/public/media
    depends_on: [mariadb, redis]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:8080/health"]
      interval: 10s
      timeout: 20s
      retries: 10
      start_period: 30s
  mariadb:
    image: mariadb:11.2
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQL
      MYSQL_DATABASE: castopod
      MYSQL_USER: $SERVICE_USER_MYSQL
      MYSQL_PASSWORD: $SERVICE_PASSWORD_MYSQL
    volumes:
      - castopod_db:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 5s
      timeout: 20s
      retries: 10
  redis:
    image: redis:7.2-alpine
    environment:
      REDIS_PASSWORD: $SERVICE_PASSWORD_REDIS
    command: ["/bin/sh", "-c", "exec redis-server --requirepass \"$REDIS_PASSWORD\""]
    volumes:
      - castopod_cache:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli -a \"$REDIS_PASSWORD\" ping | grep -q PONG || exit 1"]
      interval: 5s
      timeout: 20s
      retries: 10
`,
	},
	{ //nolint:gosec // ADMIN_PASSWORD/DB_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "matrix-synapse-postgres",
		Name:                   "Matrix Synapse (Postgres)",
		Slogan:                 "A Matrix homeserver backed by Postgres, for self-hosted federated chat and voice/video signaling.",
		Category:               "Communication",
		DocumentationURL:       "https://element-hq.github.io/synapse/latest/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  synapse:
    image: matrixdotorg/synapse:latest
    ports: ["8008:8008"]
    environment:
      SYNAPSE_SERVER_NAME: localhost
      SYNAPSE_REPORT_STATS: "no"
      SYNAPSE_PUBLIC_BASEURL: ${SERVICE_FQDN_SYNAPSE:-http://localhost:8008}
      ENABLE_REGISTRATION: "false"
      ADMIN_USERNAME: $SERVICE_USER_ADMIN
      ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      DB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - synapse_data:/data
    depends_on: [postgres]
    # The image's own ENTRYPOINT is /start.py, which treats its first
    # argument as a fixed execution mode ("generate", "migrate_config",
    # ...) and rejects anything else; entrypoint: (not command:) is the
    # only way to replace it with a real shell. /start.py generate
    # itself writes homeserver.yaml and a signing key named after
    # SYNAPSE_SERVER_NAME; this overwrites that config with a
    # Postgres-pointed one but keeps the generated signing key path, and
    # reuses DB_PASSWORD for Synapse's own internal secrets rather than
    # scraping them back out of the first-generated file.
    entrypoint:
      - /bin/sh
      - -c
      - |
        set -e
        if [ ! -f /data/homeserver.yaml ]; then
          /start.py generate
        fi
        cat > /data/homeserver.yaml <<CONF
        server_name: "$SYNAPSE_SERVER_NAME"
        pid_file: /data/homeserver.pid
        public_baseurl: "$SYNAPSE_PUBLIC_BASEURL/"
        listeners:
          - port: 8008
            tls: false
            type: http
            x_forwarded: true
            bind_addresses: ['0.0.0.0']
            resources:
              - names: [client, federation]
                compress: false
        database:
          name: psycopg2
          args:
            user: synapse
            password: $DB_PASSWORD
            database: synapse
            host: postgres
            port: 5432
            cp_min: 5
            cp_max: 10
        log_config: "/data/$SYNAPSE_SERVER_NAME.log.config"
        media_store_path: /data/media_store
        report_stats: $SYNAPSE_REPORT_STATS
        registration_shared_secret: $DB_PASSWORD
        macaroon_secret_key: $DB_PASSWORD
        form_secret: $DB_PASSWORD
        signing_key_path: "/data/$SYNAPSE_SERVER_NAME.signing.key"
        trusted_key_servers:
          - server_name: "matrix.org"
        CONF
        if [ "$ENABLE_REGISTRATION" = "true" ]; then
          echo "enable_registration: true" >> /data/homeserver.yaml
        fi
        if [ -n "$ADMIN_USERNAME" ]; then
          (
            for i in $(seq 1 60); do
              curl -sf http://localhost:8008/health >/dev/null 2>&1 && break
              sleep 2
            done
            register_new_matrix_user -a -u "$ADMIN_USERNAME" -p "$ADMIN_PASSWORD" -c /data/homeserver.yaml http://localhost:8008 >/dev/null 2>&1
          ) &
        fi
        exec /start.py
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:8008/health"]
      interval: 10s
      timeout: 5s
      retries: 15
      start_period: 30s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: synapse
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: synapse
      POSTGRES_INITDB_ARGS: --encoding=UTF8 --lc-collate=C --lc-ctype=C
    volumes:
      - synapse_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U synapse -d synapse"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // UNLEASH_DEFAULT_ADMIN_PASSWORD/POSTGRES_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "unleash-postgres",
		Name:                   "Unleash (Postgres)",
		Slogan:                 "An open-source feature flag platform with gradual rollouts, A/B testing, and a permission model.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.getunleash.io/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  unleash:
    image: unleashorg/unleash-server:latest
    ports: ["4242:4242"]
    environment:
      UNLEASH_URL: ${SERVICE_FQDN_UNLEASH:-http://localhost:4242}
      UNLEASH_DEFAULT_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      DATABASE_URL: postgres://unleash:$SERVICE_PASSWORD_DB@postgres/unleash
      DATABASE_SSL: "false"
      LOG_LEVEL: warn
    depends_on: [postgres]
    healthcheck:
      test: ["CMD-SHELL", "wget --no-verbose --tries=1 --spider http://127.0.0.1:4242/health || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 20s
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: unleash
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: unleash
    volumes:
      - unleash_postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U unleash -d unleash"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "libreoffice",
		Name:                   "LibreOffice (Remote Desktop)",
		Slogan:                 "A full LibreOffice desktop running in a container, reachable from any browser, for editing office documents.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.libreoffice.org/discover/libreoffice/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  libreoffice:
    image: lscr.io/linuxserver/libreoffice:latest
    ports: ["3000:3000"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: Etc/UTC
    volumes:
      - libreoffice_config:/config
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://127.0.0.1:3000"]
      interval: 15s
      timeout: 10s
      retries: 10
      start_period: 30s
`,
	},
	{ //nolint:gosec // MYSQL_ROOT_PASSWORD/GLPI_DB_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "glpi",
		Name:                   "GLPI",
		Slogan:                 "An IT asset and service-desk management suite: inventory, tickets, and a CMDB in one app.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://glpi-project.org/documentation/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  glpi:
    image: glpi/glpi:11
    ports: ["8080:80"]
    environment:
      GLPI_DB_HOST: db
      GLPI_DB_PORT: "3306"
      GLPI_DB_NAME: glpi
      GLPI_DB_USER: glpi
      GLPI_DB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - glpi_data:/var/glpi
    depends_on: [db]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1/"]
      interval: 15s
      timeout: 10s
      retries: 10
      start_period: 30s
  db:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
      MYSQL_DATABASE: glpi
      MYSQL_USER: glpi
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - glpi_db_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1"]
      interval: 5s
      timeout: 10s
      retries: 10
`,
	},
	{ //nolint:gosec // MARIADB_ROOT_PASSWORD/ADMIN_PASS below are compose magic-var tokens, not real credentials
		ID:                     "freescout",
		Name:                   "FreeScout",
		Slogan:                 "A free, self-hosted help desk and shared mailbox, a lighter alternative to Zendesk or Help Scout.",
		Category:               "Communication",
		DocumentationURL:       "https://github.com/freescout-helpdesk/freescout/wiki",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  freescout:
    image: tiredofit/freescout:latest
    ports: ["8080:80"]
    environment:
      DB_HOST: mariadb
      DB_NAME: freescout
      DB_USER: freescout
      DB_PASS: $SERVICE_PASSWORD_DB
      SITE_URL: ${SERVICE_FQDN_FREESCOUT:-http://localhost:8080}
      ADMIN_EMAIL: admin@example.com
      ADMIN_PASS: $SERVICE_PASSWORD_ADMIN
      DISPLAY_ERRORS: "FALSE"
      TIMEZONE: UTC
    volumes:
      - freescout_data:/data
      - freescout_logs:/www/logs
    depends_on: [mariadb]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1/"]
      interval: 10s
      timeout: 10s
      retries: 15
      start_period: 15s
  mariadb:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
      MARIADB_DATABASE: freescout
      MARIADB_USER: freescout
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - freescout_db_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 5s
      timeout: 10s
      retries: 15
      start_period: 15s
`,
	},
	{ //nolint:gosec // MYSQL_ROOT_PASSWORD/ORANGEHRM_DATABASE_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "orangehrm",
		Name:                   "OrangeHRM",
		Slogan:                 "An open-source human resources management suite: PTO, recruitment, performance, and an employee directory.",
		Category:               "Applications",
		DocumentationURL:       "https://docs.orangehrm.com/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		// Upstream's own template pins platform: linux/amd64; this
		// platform's compose subset has no platform: field, so an arm64
		// host will try (and may fail) to pull a native image instead.
		Compose: `services:
  orangehrm:
    image: orangehrm/orangehrm:latest
    ports: ["8080:80"]
    environment:
      ORANGEHRM_DATABASE_HOST: mariadb
      ORANGEHRM_DATABASE_USER: orangehrm
      ORANGEHRM_DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
      ORANGEHRM_DATABASE_NAME: orangehrm
    volumes:
      - orangehrm_data:/orangehrm
    depends_on: [mariadb]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1/"]
      interval: 10s
      timeout: 10s
      retries: 10
      start_period: 30s
  mariadb:
    image: mariadb:11
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
      MYSQL_DATABASE: orangehrm
      MYSQL_USER: orangehrm
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - orangehrm_mariadb_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 5s
      timeout: 10s
      retries: 10
`,
	},
	{ //nolint:gosec // MYSQL_ROOT_PASSWORD/DB_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "easyappointments",
		Name:                   "Easy!Appointments",
		Slogan:                 "A self-hosted appointment scheduling app with a public booking page, working hours, and reminders.",
		Category:               "Productivity",
		DocumentationURL:       "https://easyappointments.org/docs.html",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  easyappointments:
    image: alextselegidis/easyappointments:latest
    ports: ["8080:80"]
    environment:
      BASE_URL: ${SERVICE_FQDN_EASYAPPOINTMENTS:-http://localhost:8080}
      DB_HOST: mysql
      DB_NAME: easyappointments
      DB_USERNAME: root
      DB_PASSWORD: $SERVICE_PASSWORD_DB
    depends_on: [mysql]
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1/"]
      interval: 10s
      timeout: 10s
      retries: 20
      start_period: 15s
  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_DATABASE: easyappointments
    volumes:
      - easyappointments_mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1"]
      interval: 5s
      timeout: 10s
      retries: 10
`,
	},
	{
		ID:                     "cockpit-cms",
		Name:                   "Cockpit",
		Slogan:                 "A lightweight headless content management platform with a flexible content modeler and a REST/GraphQL API.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://getcockpit.com/documentation",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  cockpit:
    image: cockpithq/cockpit:core-latest
    ports: ["8080:80"]
    volumes:
      - cockpit_config:/var/www/html/config
      - cockpit_spaces:/var/www/html/.spaces
      - cockpit_storage:/var/www/html/storage
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://127.0.0.1/api/system/healthcheck"]
      interval: 10s
      timeout: 10s
      retries: 10
      start_period: 20s
`,
	},
	{ //nolint:gosec // TOKEN below is a compose magic-var token, not a real credential
		ID:                     "browserless",
		Name:                   "Browserless",
		Slogan:                 "A headless Chromium-as-a-service for scraping, PDF generation, and screenshot automation over a REST API.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.browserless.io/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  browserless:
    image: ghcr.io/browserless/chromium:latest
    ports: ["3000:3000"]
    environment:
      TOKEN: $SERVICE_PASSWORD_BROWSERLESS
    healthcheck:
      test: ["CMD", "curl", "-f", "http://127.0.0.1:3000/docs"]
      interval: 10s
      timeout: 10s
      retries: 10
      start_period: 15s
`,
	},
	{ //nolint:gosec // ADMIN_KEY below is a compose magic-var token, not a real credential
		ID:                     "cap-captcha",
		Name:                   "Cap",
		Slogan:                 "A lightweight, privacy-friendly proof-of-work CAPTCHA, an alternative to reCAPTCHA or hCaptcha.",
		Category:               "Security",
		DocumentationURL:       "https://capjs.js.org/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  cap:
    image: tiago2/cap:3.0.4
    ports: ["3000:3000"]
    environment:
      ADMIN_KEY: $SERVICE_PASSWORD_ADMIN
      REDIS_URL: redis://valkey:6379
    depends_on: [valkey]
    healthcheck:
      test: ["CMD", "bun", "-e", "fetch('http://localhost:3000').then(r => { if (!r.ok) process.exit(1) }).catch(() => process.exit(1))"]
      interval: 30s
      timeout: 5s
      retries: 10
      start_period: 10s
  valkey:
    image: valkey/valkey:9-alpine
    volumes:
      - cap_valkey_data:/data
    command: ["valkey-server", "--save", "60", "1", "--loglevel", "warning", "--maxmemory-policy", "noeviction"]
    healthcheck:
      test: ["CMD", "valkey-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
}
