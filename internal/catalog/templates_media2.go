package catalog

var media2Templates = []Template{
	{
		ID:                     "photoprism",
		Name:                   "PhotoPrism",
		Slogan:                 "An AI-powered photo management app that indexes and organizes your library as you own it.",
		Category:               "Media",
		DocumentationURL:       "https://docs.photoprism.app/getting-started/docker-compose/",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		// PhotoPrism's official releases use a date-based tag scheme
		// (YYMMDD) rather than semver.
		Compose: `services:
  photoprism:
    image: photoprism/photoprism:260728
    ports: ["2342:2342"]
    environment:
      PHOTOPRISM_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      PHOTOPRISM_SITE_URL: ${SERVICE_FQDN_PHOTOPRISM:-http://localhost:2342}
      PHOTOPRISM_DISABLE_TLS: "true"
    volumes:
      - photoprism_originals:/photoprism/originals
      - photoprism_storage:/photoprism/storage
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:2342/api/v1/status"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "planka",
		Name:                   "Planka",
		Slogan:                 "A Trello-style kanban board for visualizing and tracking work across a team.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.planka.cloud/docs/installation/docker/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  planka:
    image: ghcr.io/plankanban/planka:2.2.1
    ports: ["3000:1337"]
    environment:
      BASE_URL: ${SERVICE_FQDN_PLANKA:-http://localhost:3000}
      SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      DATABASE_URL: postgresql://postgres:$SERVICE_PASSWORD_DB@db/planka
    volumes:
      - planka_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:1337/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: planka
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - planka_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "kavita",
		Name:                   "Kavita",
		Slogan:                 "A fast, feature-rich reader server for manga, comics, and ebooks.",
		Category:               "Media",
		DocumentationURL:       "https://wiki.kavitareader.com/installation/docker/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  kavita:
    image: jvmilazz0/kavita:0.9.1
    ports: ["5000:5000"]
    volumes:
      - kavita_config:/kavita/config
      - kavita_data:/manga
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5000/api/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "komga",
		Name:                   "Komga",
		Slogan:                 "A media server for comics, manga, and digital books with a clean reading interface.",
		Category:               "Media",
		DocumentationURL:       "https://komga.org/docs/installation/docker",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  komga:
    image: gotson/komga:1.26.3
    ports: ["25600:25600"]
    volumes:
      - komga_config:/config
      - komga_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:25600/actuator/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
`,
	},
	{
		ID:                     "lidarr",
		Name:                   "Lidarr",
		Slogan:                 "Watches your indexers for new albums from artists you follow and automatically grabs and organizes them.",
		Category:               "Media",
		DocumentationURL:       "https://wiki.servarr.com/lidarr",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  lidarr:
    image: lscr.io/linuxserver/lidarr:2.9.6
    ports: ["8686:8686"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - lidarr_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8686/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "bazarr",
		Name:                   "Bazarr",
		Slogan:                 "Companion to Sonarr and Radarr that finds and downloads subtitles for your media library.",
		Category:               "Media",
		DocumentationURL:       "https://wiki.bazarr.media",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  bazarr:
    image: lscr.io/linuxserver/bazarr:1.5.1
    ports: ["6767:6767"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - bazarr_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:6767/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "jackett",
		Name:                   "Jackett",
		Slogan:                 "A proxy that translates queries from your media apps into torrent tracker searches.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/Jackett/Jackett#readme",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  jackett:
    image: lscr.io/linuxserver/jackett:0.24.2663
    ports: ["9117:9117"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - jackett_config:/config
      - jackett_downloads:/downloads
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9117/UI/Dashboard"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
}
