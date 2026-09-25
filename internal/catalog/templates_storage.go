package catalog

var storageTemplates = []Template{
	{
		ID:                     "minio",
		Name:                   "MinIO",
		Slogan:                 "S3-compatible object storage you run yourself, with a built-in web console.",
		Category:               "Storage",
		DocumentationURL:       "https://min.io/docs/minio/linux/index.html",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Real MinIO images require a "server /data" style command to
		// actually serve.
		Compose: `services:
  minio:
    image: minio/minio:RELEASE.2024-10-13T13-34-11Z
    command: ["server", "/data", "--console-address", ":9001"]
    ports: ["9000:9000", "9001:9001"]
    environment:
      MINIO_ROOT_USER: $SERVICE_USER_ROOT
      MINIO_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
    volumes:
      - minio_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:9000/minio/health/live || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "filebrowser",
		Name:                   "File Browser",
		Slogan:                 "A simple web UI for browsing, uploading, and sharing files from your own storage.",
		Category:               "Storage",
		DocumentationURL:       "https://filebrowser.org",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct. Filebrowser normally serves
		// a bind-mounted host directory; this template still starts
		// pointed at an empty named volume, since a template can't know
		// an operator's real host paths ahead of time, but a real
		// directory can now be bind-mounted onto filebrowser_data by
		// redeploying via compose with an absolute host path in
		// volumes: (root ability required, internal/compose's own doc
		// comment on volumes:).
		Compose: `services:
  filebrowser:
    image: filebrowser/filebrowser:v2.31.2
    ports: ["8080:80"]
    volumes:
      - filebrowser_data:/srv
      - filebrowser_db:/database
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "syncthing",
		Name:                   "Syncthing",
		Slogan:                 "Continuous, peer-to-peer file synchronization between your own devices, no cloud in between.",
		Category:               "Storage",
		DocumentationURL:       "https://docs.syncthing.net",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct. Syncthing normally syncs a
		// bind-mounted host directory; this template still starts
		// pointed at an empty named volume, since a template can't know
		// an operator's real host paths ahead of time, but a real
		// directory can now be bind-mounted onto syncthing_data by
		// redeploying via compose with an absolute host path in
		// volumes: (root ability required, internal/compose's own doc
		// comment on volumes:).
		Compose: `services:
  syncthing:
    image: lscr.io/linuxserver/syncthing:1.29.4
    ports: ["8384:8384", "22000:22000"]
    volumes:
      - syncthing_config:/config
      - syncthing_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8384/rest/noauth/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "duplicati",
		Name:                   "Duplicati",
		Slogan:                 "Scheduled, encrypted backups of your files to local storage, network shares, or cloud storage.",
		Category:               "Storage",
		DocumentationURL:       "https://duplicati.readthedocs.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// linuxserver only publishes this image under a rolling :latest
		// tag; this pinned version couldn't be verified against a live
		// registry in this environment.
		Compose: `services:
  duplicati:
    image: lscr.io/linuxserver/duplicati:2.1.1.0
    ports: ["8200:8200"]
    environment:
      PUID: "1000"
      PGID: "1000"
      TZ: "Etc/UTC"
      SETTINGS_ENCRYPTION_KEY: $SERVICE_PASSWORD_ENCRYPT
      DUPLICATI__WEBSERVICE_PASSWORD: $SERVICE_PASSWORD_WEB
    volumes:
      - duplicati_config:/config
      - duplicati_backups:/backups
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8200/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "formbricks",
		Name:                   "Formbricks",
		Slogan:                 "An open-source survey and experience-management platform you run on your own infrastructure.",
		Category:               "Productivity",
		DocumentationURL:       "https://formbricks.com/docs/self-hosting/setup/docker",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  formbricks:
    image: ghcr.io/formbricks/formbricks:4.5.0
    ports: ["3000:3000"]
    environment:
      WEBAPP_URL: ${SERVICE_FQDN_FORMBRICKS:-http://localhost:3000}
      NEXTAUTH_URL: ${SERVICE_FQDN_FORMBRICKS:-http://localhost:3000}
      NEXTAUTH_SECRET: $SERVICE_BASE64_NEXTAUTHSECRET
      ENCRYPTION_KEY: $SERVICE_BASE64_ENCRYPTIONKEY
      CRON_SECRET: $SERVICE_BASE64_CRONSECRET
      DATABASE_URL: postgresql://formbricks:$SERVICE_PASSWORD_DB@db:5432/formbricks
      REDIS_URL: redis://redis:6379
    volumes:
      - formbricks_uploads:/apps/web/uploads
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_USER: formbricks
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: formbricks
    volumes:
      - formbricks_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - formbricks_redis_data:/data
`,
	},
	{ //nolint:gosec // DB_CONNECTION_URI below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "infisical",
		Name:                   "Infisical",
		Slogan:                 "An open-source secrets manager to centralize API keys, database credentials, and app config.",
		Category:               "Security",
		DocumentationURL:       "https://infisical.com/docs/self-hosting/overview",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  infisical:
    image: infisical/infisical:v0.154.6
    ports: ["8080:8080"]
    environment:
      SITE_URL: ${SERVICE_FQDN_INFISICAL:-http://localhost:8080}
      ENCRYPTION_KEY: $SERVICE_PASSWORD_ENCRYPTIONKEY
      AUTH_SECRET: $SERVICE_REALBASE64_64_AUTHSECRET
      DB_CONNECTION_URI: postgres://infisical:$SERVICE_PASSWORD_DB@db:5432/infisical
      REDIS_URL: redis://redis:6379
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/api/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: infisical
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: infisical
    volumes:
      - infisical_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - infisical_redis_data:/data
`,
	},
	{
		ID:                     "sftpgo",
		Name:                   "SFTPGo",
		Slogan:                 "An SFTP, FTP/S, and WebDAV server with a web admin UI and per-user storage backends.",
		Category:               "Storage",
		DocumentationURL:       "https://docs.sftpgo.com/latest/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  sftpgo:
    image: drakkan/sftpgo:v2.6.6
    ports: ["8080:8080"]
    volumes:
      - sftpgo_data:/srv/sftpgo
      - sftpgo_home:/var/lib/sftpgo
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
}
