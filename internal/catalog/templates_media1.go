package catalog

var media1Templates = []Template{
	{
		ID:                     "jellyfin",
		Name:                   "Jellyfin",
		Slogan:                 "A free media server for streaming your own movies, shows, and music to any device.",
		Category:               "Media",
		DocumentationURL:       "https://jellyfin.org/docs/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Jellyfin normally plays back files from a bind-mounted media
		// library; this template still starts with an empty named
		// volume, since a template can't know an operator's real host
		// paths ahead of time, but a real media directory can now be
		// bind-mounted onto jellyfin_media by redeploying via compose
		// with an absolute host path in volumes: (root ability
		// required, internal/compose's own doc comment on volumes:).
		Compose: `services:
  jellyfin:
    image: lscr.io/linuxserver/jellyfin:10.11.8
    ports: ["8096:8096"]
    volumes:
      - jellyfin_config:/config
      - jellyfin_media:/data/media
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8096/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "qbittorrent",
		Name:                   "qBittorrent",
		Slogan:                 "A free, self-hosted BitTorrent client with a full web UI for remote download management.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/qbittorrent/qBittorrent/wiki",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct.
		Compose: `services:
  qbittorrent:
    image: lscr.io/linuxserver/qbittorrent:5.0.3
    ports: ["8080:8080", "6881:6881"]
    volumes:
      - qbittorrent_config:/config
      - qbittorrent_downloads:/downloads
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "immich",
		Name:                   "Immich",
		Slogan:                 "Self-hosted photo and video backup with mobile apps, facial recognition, and timeline search.",
		Category:               "Media",
		DocumentationURL:       "https://immich.app/docs",
		RecommendedMemoryBytes: 1610612736, // 1536Mi
		// Tag not verified against a live registry in this environment;
		// the image repository and major line are correct. Pins to a
		// specific release rather than upstream's own floating
		// "release" default, since this compose subset has no way to
		// override an env default per deploy yet.
		Compose: `services:
  immich-server:
    image: ghcr.io/immich-app/immich-server:v1.126.1
    ports: ["2283:2283"]
    environment:
      DB_HOSTNAME: db
      DB_USERNAME: immich
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      DB_DATABASE_NAME: immich
      REDIS_HOSTNAME: redis
    volumes:
      - immich_upload_data:/usr/src/app/upload
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:2283/api/server/ping || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  immich-machine-learning:
    image: ghcr.io/immich-app/immich-machine-learning:v1.126.1
    volumes:
      - immich_ml_cache:/cache
  redis:
    image: redis:7.4-alpine
  db:
    image: ghcr.io/immich-app/postgres:14-vectorchord0.3.0-pgvectors0.2.0
    environment:
      POSTGRES_USER: immich
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: immich
    volumes:
      - immich_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "audiobookshelf",
		Name:                   "Audiobookshelf",
		Slogan:                 "A self-hosted server for your audiobooks and podcasts, with sync across every device.",
		Category:               "Media",
		DocumentationURL:       "https://www.audiobookshelf.org/docs",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  audiobookshelf:
    image: ghcr.io/advplyr/audiobookshelf:2.34.0
    ports: ["80:80"]
    environment:
      TZ: "Etc/UTC"
    volumes:
      - audiobookshelf_config:/config
      - audiobookshelf_metadata:/metadata
      - audiobookshelf_audiobooks:/audiobooks
      - audiobookshelf_podcasts:/podcasts
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/healthcheck || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "calcom",
		Name:                   "Cal.com",
		Slogan:                 "Open-source scheduling infrastructure for booking meetings without the back-and-forth.",
		Category:               "Productivity",
		DocumentationURL:       "https://cal.com/docs/self-hosting/installation",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		// calcom/cal.com's own registry doesn't publish a clean semver
		// tag list; this tag's exact string couldn't be verified against
		// a live registry in this environment.
		Compose: `services:
  calcom:
    image: calcom/cal.com:v5.0.9
    ports: ["3000:3000"]
    environment:
      NEXT_PUBLIC_WEBAPP_URL: ${SERVICE_FQDN_CALCOM:-http://localhost:3000}
      NEXTAUTH_URL: ${SERVICE_FQDN_CALCOM:-http://localhost:3000}
      NEXTAUTH_SECRET: $SERVICE_BASE64_NEXTAUTHSECRET
      CALENDSO_ENCRYPTION_KEY: $SERVICE_BASE64_ENCRYPTIONKEY
      DATABASE_URL: postgresql://calcom:$SERVICE_PASSWORD_DB@db:5432/calcom
      CALCOM_TELEMETRY_DISABLED: "1"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 180s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: calcom
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: calcom
    volumes:
      - calcom_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "calibre-web",
		Name:                   "Calibre-Web",
		Slogan:                 "A clean web interface for browsing, reading, and downloading your existing Calibre ebook library.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/janeczku/calibre-web/wiki",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  calibre-web:
    image: lscr.io/linuxserver/calibre-web:0.6.24
    ports: ["8083:8083"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - calibreweb_config:/config
      - calibreweb_books:/books
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8083/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "navidrome",
		Name:                   "Navidrome",
		Slogan:                 "Stream your own music collection from a Subsonic-compatible server to any device, anywhere.",
		Category:               "Media",
		DocumentationURL:       "https://www.navidrome.org/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  navidrome:
    image: deluan/navidrome:0.54.1
    ports: ["4533:4533"]
    environment:
      ND_SCANSCHEDULE: "1h"
      ND_LOGLEVEL: "info"
    volumes:
      - navidrome_data:/data
      - navidrome_music:/music
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:4533/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "overseerr",
		Name:                   "Overseerr",
		Slogan:                 "Lets your Plex users request new movies and TV shows straight from a shared web UI.",
		Category:               "Media",
		DocumentationURL:       "https://docs.overseerr.dev",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  overseerr:
    image: sctx/overseerr:1.34.0
    ports: ["5055:5055"]
    environment:
      TZ: "Etc/UTC"
    volumes:
      - overseerr_config:/app/config
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:5055/api/v1/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "prowlarr",
		Name:                   "Prowlarr",
		Slogan:                 "An indexer manager that syncs your torrent and Usenet indexers across the whole Arr stack.",
		Category:               "Media",
		DocumentationURL:       "https://wiki.servarr.com/prowlarr",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  prowlarr:
    image: lscr.io/linuxserver/prowlarr:1.22.1
    ports: ["9696:9696"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - prowlarr_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9696/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "radarr",
		Name:                   "Radarr",
		Slogan:                 "Watches your favorite indexers for movies and automatically grabs, sorts, and renames them.",
		Category:               "Media",
		DocumentationURL:       "https://wiki.servarr.com/radarr",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  radarr:
    image: lscr.io/linuxserver/radarr:5.16.2
    ports: ["7878:7878"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - radarr_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:7878/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "sonarr",
		Name:                   "Sonarr",
		Slogan:                 "Watches your favorite indexers for new TV episodes and automatically grabs, sorts, and renames them.",
		Category:               "Media",
		DocumentationURL:       "https://wiki.servarr.com/sonarr",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  sonarr:
    image: lscr.io/linuxserver/sonarr:4.0.12
    ports: ["8989:8989"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
    volumes:
      - sonarr_config:/config
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8989/ping"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "transmission",
		Name:                   "Transmission",
		Slogan:                 "A fast, lightweight BitTorrent client with a simple web interface.",
		Category:               "Media",
		DocumentationURL:       "https://docs.linuxserver.io/images/docker-transmission/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  transmission:
    image: lscr.io/linuxserver/transmission:4.0.6
    ports: ["9091:9091"]
    environment:
      PUID: "1000"
      PGID: "1000"
      USER: $SERVICE_USER_ADMIN
      PASS: $SERVICE_PASSWORD_ADMIN
    volumes:
      - transmission_config:/config
      - transmission_downloads:/downloads
      - transmission_watch:/watch
    healthcheck:
      test: ["CMD-SHELL", "sh -c ': < /dev/tcp/127.0.0.1/9091' || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "grimmory",
		Name:                   "Grimmory",
		Slogan:                 "Organize, read, annotate, and sync your entire book collection from one place.",
		Category:               "Media",
		DocumentationURL:       "https://github.com/grimmory-tools/grimmory",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  grimmory:
    image: grimmory/grimmory:nightly-20260403-3a371f7
    ports: ["80:80"]
    environment:
      DATABASE_URL: jdbc:mariadb://db:3306/grimmory
      DATABASE_USERNAME: $SERVICE_USER_DB
      DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - grimmory_data:/app/data
      - grimmory_books:/books
    healthcheck:
      test: ["CMD-SHELL", "sh -c ': < /dev/tcp/127.0.0.1/80' || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  db:
    image: mariadb:11
    environment:
      MARIADB_USER: $SERVICE_USER_DB
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_DBROOT
      MARIADB_DATABASE: grimmory
    volumes:
      - grimmory_db_data:/var/lib/mysql
`,
	},
}
