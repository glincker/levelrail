package catalog

var productivity2Templates = []Template{
	{
		ID:                     "wikijs",
		Name:                   "Wiki.js",
		Slogan:                 "A modern, extensible wiki engine with Markdown, visual editing, and fine-grained page permissions.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.requarks.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  wiki:
    image: ghcr.io/requarks/wiki:2
    ports: ["3000:3000"]
    environment:
      DB_TYPE: postgres
      DB_HOST: db
      DB_PORT: "5432"
      DB_USER: wikijs
      DB_PASS: $SERVICE_PASSWORD_DB
      DB_NAME: wikijs
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: wikijs
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: wikijs
    volumes:
      - wikijs_db_data:/var/lib/postgresql/data
`,
	},
	{ //nolint:gosec // CORE_DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "zipline",
		Name:                   "Zipline",
		Slogan:                 "A self-hosted file and screenshot host with a share-first upload flow and its own URL shortener.",
		Category:               "Storage",
		DocumentationURL:       "https://zipline.diced.sh/docs",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  zipline:
    image: ghcr.io/diced/zipline:3.7.9
    ports: ["3000:3000"]
    environment:
      CORE_RETURN_HTTPS: "false"
      CORE_DATABASE_URL: postgres://zipline:$SERVICE_PASSWORD_DB@db:5432/zipline
      CORE_SECRET: $SERVICE_HEX_64_SECRET
    volumes:
      - zipline_uploads:/zipline/uploads
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: zipline
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: zipline
    volumes:
      - zipline_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "memos",
		Name:                   "Memos",
		Slogan:                 "A lightweight, privacy-first note-taking service for jotting down quick thoughts.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.usememos.com/docs",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  memos:
    image: neosmemo/memos:stable
    ports: ["5230:5230"]
    volumes:
      - memos_data:/var/opt/memos
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5230/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "leantime",
		Name:                   "Leantime",
		Slogan:                 "A goals-focused project management tool built for people who aren't professional project managers.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.leantime.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  leantime:
    image: leantime/leantime:3.5.6
    ports: ["8080:8080"]
    environment:
      LEAN_DB_HOST: db
      LEAN_DB_USER: leantime
      LEAN_DB_PASSWORD: $SERVICE_PASSWORD_DB
      LEAN_DB_DATABASE: leantime
      LEAN_SESSION_PASSWORD: $SERVICE_PASSWORD_64_SESSION
      LEAN_USE_REDIS: "true"
      LEAN_REDIS_HOST: redis
      LEAN_REDIS_PORT: "6379"
    volumes:
      - leantime_userfiles:/var/www/html/userfiles
      - leantime_public_userfiles:/var/www/html/public/userfiles
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: mysql:8.4
    environment:
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MYSQLROOT
      MYSQL_USER: leantime
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_DATABASE: leantime
    volumes:
      - leantime_db_data:/var/lib/mysql
  redis:
    image: redis:7-alpine
    volumes:
      - leantime_redis_data:/data
`,
	},
	{
		ID:                     "osticket",
		Name:                   "osTicket",
		Slogan:                 "A widely used open-source support ticket system for teams handling customer requests.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.osticket.com/en/latest/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  osticket:
    image: tiredofit/osticket:1.18.2
    ports: ["80:80"]
    environment:
      DB_HOST: db
      DB_NAME: osticket
      DB_USER: osticket
      DB_PASS: $SERVICE_PASSWORD_DB
      INSTALL_SECRET: $SERVICE_PASSWORD_INSTALLSECRET
      ADMIN_EMAIL: admin@example.com
      ADMIN_USER: $SERVICE_USER_ADMIN
      ADMIN_PASS: $SERVICE_PASSWORD_ADMIN
    volumes:
      - osticket_data:/www/osticket
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 180s
  db:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_DBROOT
      MARIADB_DATABASE: osticket
      MARIADB_USER: osticket
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - osticket_db_data:/var/lib/mysql
`,
	},
	{
		ID:                     "penpot",
		Name:                   "Penpot",
		Slogan:                 "An open-source design and prototyping platform, a self-hosted alternative to Figma.",
		Category:               "Productivity",
		DocumentationURL:       "https://help.penpot.app",
		RecommendedMemoryBytes: 2147483648, // 2048Mi
		Compose: `services:
  frontend:
    image: penpotapp/frontend:2.11.1
    ports: ["8080:8080"]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  backend:
    image: penpotapp/backend:2.11.1
    environment:
      PENPOT_FLAGS: "enable-login-with-password disable-email-verification"
      PENPOT_SECRET_KEY: $SERVICE_REALBASE64_64_SECRETKEY
      PENPOT_PUBLIC_URI: ${SERVICE_FQDN_FRONTEND:-http://localhost:8080}
      PENPOT_DATABASE_URI: postgresql://postgres/penpot
      PENPOT_DATABASE_USERNAME: $SERVICE_USER_POSTGRES
      PENPOT_DATABASE_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      PENPOT_REDIS_URI: redis://valkey/0
      PENPOT_OBJECTS_STORAGE_BACKEND: fs
      PENPOT_OBJECTS_STORAGE_FS_DIRECTORY: /opt/data/assets
    volumes:
      - penpot_assets:/opt/data/assets
  exporter:
    image: penpotapp/exporter:2.11.1
    environment:
      PENPOT_PUBLIC_URI: ${SERVICE_FQDN_FRONTEND:-http://localhost:8080}
      PENPOT_REDIS_URI: redis://valkey/0
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_INITDB_ARGS: "--data-checksums"
      POSTGRES_USER: $SERVICE_USER_POSTGRES
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_POSTGRES
      POSTGRES_DB: penpot
    volumes:
      - penpot_db_data:/var/lib/postgresql/data
  valkey:
    image: valkey/valkey:8.1-alpine
    volumes:
      - penpot_valkey_data:/data
`,
	},
	{
		ID:                     "grist",
		Name:                   "Grist",
		Slogan:                 "A modern relational spreadsheet that combines spreadsheet flexibility with database structure.",
		Category:               "Productivity",
		DocumentationURL:       "https://support.getgrist.com",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  grist:
    image: gristlabs/grist:1.3.3
    ports: ["8484:8484"]
    environment:
      TYPEORM_TYPE: postgres
      TYPEORM_HOST: db
      TYPEORM_DATABASE: grist
      TYPEORM_USERNAME: $SERVICE_USER_DB
      TYPEORM_PASSWORD: $SERVICE_PASSWORD_DB
      REDIS_URL: redis://redis:6379
      GRIST_SESSION_SECRET: $SERVICE_REALBASE64_64_SESSIONSECRET
    volumes:
      - grist_data:/persist
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8484/status"]
      interval: 10s
      timeout: 5s
      retries: 3
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: grist
    volumes:
      - grist_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - grist_redis_data:/data
`,
	},
	{
		ID:                     "readeck",
		Name:                   "Readeck",
		Slogan:                 "Save the readable content of web pages you want to keep, free of ads and clutter.",
		Category:               "Productivity",
		DocumentationURL:       "https://readeck.org/en/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  readeck:
    image: codeberg.org/readeck/readeck:0.19.1
    ports: ["8000:8000"]
    volumes:
      - readeck_data:/readeck
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "linkding",
		Name:                   "Linkding",
		Slogan:                 "A minimal, fast bookmark manager built for keeping a personal link archive.",
		Category:               "Productivity",
		DocumentationURL:       "https://linkding.link",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  linkding:
    image: sissbruecker/linkding:1.31.0
    ports: ["9090:9090"]
    environment:
      LD_SUPERUSER_NAME: $SERVICE_USER_ADMIN
      LD_SUPERUSER_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - linkding_data:/etc/linkding/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9090/health"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // CMD_DB_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "hedgedoc",
		Name:                   "HedgeDoc",
		Slogan:                 "Real-time collaborative markdown notes you can host yourself.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.hedgedoc.org",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  hedgedoc:
    image: quay.io/hedgedoc/hedgedoc:1.9.9
    ports: ["3000:3000"]
    environment:
      CMD_DOMAIN: ${SERVICE_FQDN_HEDGEDOC:-localhost}
      CMD_URL_ADDPORT: "false"
      CMD_DB_URL: postgres://$SERVICE_USER_DB:$SERVICE_PASSWORD_DB@db:5432/hedgedoc
      CMD_SESSION_SECRET: $SERVICE_HEX_64_SESSIONSECRET
    volumes:
      - hedgedoc_uploads:/hedgedoc/public/uploads
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/status"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: hedgedoc
    volumes:
      - hedgedoc_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "kanboard",
		Name:                   "Kanboard",
		Slogan:                 "A minimalist, keyboard-friendly kanban board for personal and team task tracking.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.kanboard.org",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  kanboard:
    image: kanboard/kanboard:v1.2.54
    ports: ["80:80"]
    volumes:
      - kanboard_data:/var/www/app/data
      - kanboard_plugins:/var/www/app/plugins
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "wallabag",
		Name:                   "Wallabag",
		Slogan:                 "A read-it-later app that saves web articles in a clean, distraction-free format.",
		Category:               "Productivity",
		DocumentationURL:       "https://doc.wallabag.org",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  wallabag:
    image: wallabag/wallabag:2.6.14
    ports: ["80:80"]
    environment:
      SYMFONY__ENV__DOMAIN_NAME: ${SERVICE_FQDN_WALLABAG:-http://localhost}
      SYMFONY__ENV__SECRET: $SERVICE_HEX_32_SECRET
    volumes:
      - wallabag_data:/var/www/wallabag/data
      - wallabag_images:/var/www/wallabag/web/assets/images
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
}
