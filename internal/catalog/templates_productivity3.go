package catalog

var productivity3Templates = []Template{
	{
		ID:                     "tandoor-recipes",
		Name:                   "Tandoor Recipes",
		Slogan:                 "A recipe manager and meal planner with shopping lists and shared cookbooks.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.tandoor.dev/install/docker/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  tandoor:
    image: vabene1111/recipes:2.6.15
    ports: ["8080:80"]
    environment:
      SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      ALLOWED_HOSTS: "*"
      DB_ENGINE: django.db.backends.postgresql
      POSTGRES_HOST: db
      POSTGRES_PORT: "5432"
      POSTGRES_USER: tandoor
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: tandoor
    volumes:
      - tandoor_data:/opt/recipes/mediafiles
      - tandoor_static:/opt/recipes/staticfiles
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: tandoor
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: tandoor
    volumes:
      - tandoor_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "homebox",
		Name:                   "Homebox",
		Slogan:                 "A home inventory and organization system for tracking what you own and where it is.",
		Category:               "Productivity",
		DocumentationURL:       "https://homebox.software/en/quick-start/install/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  homebox:
    image: ghcr.io/sysadminsmedia/homebox:0.11.1
    ports: ["7745:7745"]
    environment:
      HBOX_OPTIONS_ALLOW_REGISTRATION: "true"
    volumes:
      - homebox_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:7745/api/v1/status"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "dokuwiki",
		Name:                   "DokuWiki",
		Slogan:                 "A simple, database-free wiki that stores pages as plain text files, easy to back up.",
		Category:               "Productivity",
		DocumentationURL:       "https://www.dokuwiki.org/manual",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  dokuwiki:
    image: lscr.io/linuxserver/dokuwiki:2024-02-06b
    ports: ["80:80"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - dokuwiki_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "etherpad",
		Name:                   "Etherpad",
		Slogan:                 "A real-time collaborative editor for documents, with plugins and a clean export story.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.etherpad.org",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  etherpad:
    image: etherpad/etherpad:2.2.4
    ports: ["9001:9001"]
    environment:
      ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      DEFAULT_PAD_TEXT: "Welcome to Etherpad."
    volumes:
      - etherpad_data:/opt/etherpad-lite/var
    healthcheck:
      test: ["CMD", "node", "-e", "require('http').get('http://127.0.0.1:9001/',r=>process.exit(r.statusCode<500?0:1)).on('error',()=>process.exit(1))"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "shiori",
		Name:                   "Shiori",
		Slogan:                 "A simple bookmark manager with offline archiving, built as a single Go binary.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/go-shiori/shiori/tree/master/docs",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  shiori:
    image: ghcr.io/go-shiori/shiori:v1.7.1
    ports: ["8080:8080"]
    volumes:
      - shiori_data:/shiori
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:8080/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
	{
		ID:                     "flatnotes",
		Name:                   "flatnotes",
		Slogan:                 "A database-less note-taking app that keeps notes as plain Markdown files.",
		Category:               "Productivity",
		DocumentationURL:       "https://github.com/dullage/flatnotes/wiki",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  flatnotes:
    image: dullage/flatnotes:v4.1.1
    ports: ["8080:8080"]
    environment:
      FLATNOTES_AUTH_TYPE: "password"
      FLATNOTES_USERNAME: admin
      FLATNOTES_PASSWORD: $SERVICE_PASSWORD_ADMIN
      FLATNOTES_SECRET_KEY: $SERVICE_PASSWORD_SECRET
    volumes:
      - flatnotes_data:/data
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/health')"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
}
