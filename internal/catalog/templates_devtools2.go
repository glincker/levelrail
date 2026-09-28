package catalog

var devtools2Templates = []Template{
	{
		ID:                     "tolgee",
		Name:                   "Tolgee",
		Slogan:                 "A localization management platform where developers and translators work in one shared UI.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://tolgee.io/platform",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  tolgee:
    image: tolgee/tolgee:v3.224.7
    ports: ["8080:8080"]
    environment:
      TOLGEE_AUTHENTICATION_ENABLED: "true"
      TOLGEE_AUTHENTICATION_INITIAL_USERNAME: admin
      TOLGEE_AUTHENTICATION_INITIAL_PASSWORD: $SERVICE_PASSWORD_ADMIN
      TOLGEE_AUTHENTICATION_JWT_SECRET: $SERVICE_HEX_64_JWTSECRET
      TOLGEE_POSTGRES_AUTOSTART_ENABLED: "false"
      SPRING_DATASOURCE_URL: jdbc:postgresql://db:5432/tolgee
      SPRING_DATASOURCE_USERNAME: $SERVICE_USER_DB
      SPRING_DATASOURCE_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - tolgee_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/actuator/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 180s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: tolgee
    volumes:
      - tolgee_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "weblate",
		Name:                   "Weblate",
		Slogan:                 "A continuous localization system for translating software with a web-based editor and review flow.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.weblate.org",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment. Weblate's redis normally needs --requirepass
		// via command: to actually enforce REDIS_PASSWORD; not added
		// here because ResolveMagicVars only substitutes SERVICE_ tokens
		// inside environment:, not command:, so $SERVICE_PASSWORD_REDIS
		// would reach the container as a literal, unresolved string. The
		// password below still passes through unenforced for now.
		Compose: `services:
  weblate:
    image: weblate/weblate:5.15
    ports: ["8080:8080"]
    environment:
      WEBLATE_SITE_DOMAIN: ${SERVICE_FQDN_WEBLATE:-localhost}
      WEBLATE_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      POSTGRES_HOST: db
      POSTGRES_USER: weblate
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DATABASE: weblate
      REDIS_HOST: redis
      REDIS_PORT: "6379"
      REDIS_PASSWORD: $SERVICE_PASSWORD_REDIS
    volumes:
      - weblate_data:/app/data
      - weblate_cache:/app/cache
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/healthz/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 180s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: weblate
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: weblate
    volumes:
      - weblate_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    environment:
      REDIS_PASSWORD: $SERVICE_PASSWORD_REDIS
    volumes:
      - weblate_redis_data:/data
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "rallly",
		Name:                   "Rallly",
		Slogan:                 "Find a time that works for everyone with polls for scheduling meetings and events.",
		Category:               "Productivity",
		DocumentationURL:       "https://support.rallly.co/self-hosting/introduction",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  rallly:
    image: lukevella/rallly:4.15.2
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://$SERVICE_USER_DB:$SERVICE_PASSWORD_DB@db:5432/rallly
      SECRET_PASSWORD: $SERVICE_HEX_64_SECRET
      NEXT_PUBLIC_BASE_URL: ${SERVICE_FQDN_RALLLY:-http://localhost:3000}
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: rallly
    volumes:
      - rallly_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "databasus",
		Name:                   "Databasus",
		Slogan:                 "A free, self-hosted backup tool for Postgres, MySQL, and MongoDB databases.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://databasus.com/installation",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  databasus:
    image: databasus/databasus:v3.16.2
    ports: ["4005:4005"]
    volumes:
      - databasus_data:/databasus-data
    healthcheck:
      test: ["CMD-SHELL", "sh -c ': < /dev/tcp/127.0.0.1/4005' || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "wakapi",
		Name:                   "Wakapi",
		Slogan:                 "A self-hosted, WakaTime-compatible backend for tracking coding time and stats.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://wakapi.dev",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  wakapi:
    image: ghcr.io/muety/wakapi:2.13.0
    ports: ["3000:3000"]
    environment:
      WAKAPI_DB_TYPE: postgres
      WAKAPI_DB_HOST: db
      WAKAPI_DB_NAME: wakapi
      WAKAPI_DB_USER: $SERVICE_USER_DB
      WAKAPI_DB_PASSWORD: $SERVICE_PASSWORD_DB
      WAKAPI_SECURITY_PASSWORD_SALT: $SERVICE_BASE64_64_PASSWORDSALT
    volumes:
      - wakapi_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api/health"]
      interval: 10s
      timeout: 5s
      retries: 3
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: wakapi
    volumes:
      - wakapi_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "gitlab-ce",
		Name:                   "GitLab CE",
		Slogan:                 "A complete DevOps platform for source control, code review, issues, and CI/CD in one place.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.gitlab.com/install/docker/installation/",
		RecommendedMemoryBytes: 4294967296, // 4096Mi
		// GitLab's omnibus image also serves SSH git access on 22 and
		// HTTPS on 443; this platform tracks a single container port per
		// service, so only the web UI on 80 is reachable here.
		Compose: `services:
  gitlab:
    image: gitlab/gitlab-ce:19.3.1-ce.0
    ports: ["80:80"]
    environment:
      GITLAB_OMNIBUS_CONFIG: |
        external_url '${SERVICE_FQDN_GITLAB:-http://localhost}'
    volumes:
      - gitlab_config:/etc/gitlab
      - gitlab_logs:/var/log/gitlab
      - gitlab_data:/var/opt/gitlab
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/-/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "typesense",
		Name:                   "Typesense",
		Slogan:                 "A fast, typo-tolerant search engine API built as a lighter alternative to Elasticsearch.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://typesense.org/docs/guide/install-typesense.html",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  typesense:
    image: typesense/typesense:30.2
    ports: ["8108:8108"]
    environment:
      TYPESENSE_API_KEY: $SERVICE_HEX_32_APIKEY
      TYPESENSE_DATA_DIR: /data
    volumes:
      - typesense_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8108/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "libretranslate",
		Name:                   "LibreTranslate",
		Slogan:                 "A free and open machine translation API that runs entirely on your own hardware.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/LibreTranslate/LibreTranslate",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  libretranslate:
    image: libretranslate/libretranslate:v1.9.6
    ports: ["5000:5000"]
    volumes:
      - libretranslate_data:/home/libretranslate/.local
    healthcheck:
      test: ["CMD-SHELL", "sh -c ': < /dev/tcp/127.0.0.1/5000' || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "forgejo",
		Name:                   "Forgejo",
		Slogan:                 "A lightweight, community-governed Git forge with issues, pull requests, and CI runners.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://forgejo.org/docs/latest/admin/installation-docker/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag unverified: codeberg.org returned 401.
		Compose: `services:
  forgejo:
    image: codeberg.org/forgejo/forgejo:9.0.3
    ports: ["3000:3000"]
    environment:
      FORGEJO__server__ROOT_URL: ${SERVICE_FQDN_FORGEJO:-http://localhost:3000}
      FORGEJO__security__SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
    volumes:
      - forgejo_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "mailpit",
		Name:                   "Mailpit",
		Slogan:                 "A local SMTP server and web inbox for catching and inspecting outgoing email during development.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://mailpit.axllent.org/docs/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  mailpit:
    image: axllent/mailpit:v1.21.8
    ports: ["8025:8025"]
    volumes:
      - mailpit_data:/data
    environment:
      MP_DATABASE: /data/mailpit.db
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8025/livez || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "pocketbase",
		Name:                   "PocketBase",
		Slogan:                 "An open-source backend in one file: embedded database, auth, file storage, and realtime API.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://pocketbase.io/docs/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  pocketbase:
    image: ghcr.io/muchobien/pocketbase:0.22.27
    ports: ["8090:8090"]
    volumes:
      - pocketbase_data:/pb_data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8090/api/health || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "cyberchef",
		Name:                   "CyberChef",
		Slogan:                 "The cyber swiss army knife: encode, decode, hash, and analyse data in the browser.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://github.com/gchq/CyberChef/wiki",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  cyberchef:
    image: ghcr.io/gchq/cyberchef:10.19.4
    ports: ["8000:80"]
    volumes:
      - cyberchef_cache:/var/cache/nginx
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
	{
		ID:                     "registry",
		Name:                   "Docker Registry",
		Slogan:                 "The official open source registry for storing and distributing your own container images.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://distribution.github.io/distribution/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  registry:
    image: registry:2.8.3
    ports: ["5000:5000"]
    volumes:
      - registry_data:/var/lib/registry
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:5000/v2/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 10s
`,
	},
	{
		ID:                     "verdaccio",
		Name:                   "Verdaccio",
		Slogan:                 "A lightweight private npm proxy registry with caching and local package publishing.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://verdaccio.org/docs/installation",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  verdaccio:
    image: verdaccio/verdaccio:5.31.1
    ports: ["4873:4873"]
    environment:
      VERDACCIO_PORT: "4873"
    volumes:
      - verdaccio_storage:/verdaccio/storage
      - verdaccio_conf:/verdaccio/conf
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:4873/-/ping || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
}
