package catalog

// devtoolsBatchTemplates adds a second wave of well-known dev-tool and
// infrastructure templates (ADR 015's Coolify-catalog import), each
// trimmed to fit this platform's one-container-per-service compose
// subset: no bind-mount file injection, no one-shot init containers, no
// depends_on service_healthy ordering. Where a source project's own real
// setup needs one of those (Supabase's config injection, Dozzle's
// docker.sock access, which internal/bindmount forbids), the entry was
// dropped rather than shipped half-working.
var devtoolsBatchTemplates = []Template{
	{ //nolint:gosec // every *_PASS/*_PASSWORD/*_SECRET value below is a compose magic-var token, not a real credential
		ID:                     "appwrite",
		Name:                   "Appwrite",
		Slogan:                 "An open-source backend-as-a-service with auth, databases, storage, and functions behind one API.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://appwrite.io/docs",
		RecommendedMemoryBytes: 2147483648, // 2048Mi
		// Upstream's real stack is 20+ containers (one per queue worker:
		// mails, webhooks, builds, functions, ...). This keeps only the
		// API, console, database, and cache, so the control plane, auth,
		// and basic database/storage features work; anything that needs a
		// background worker (outgoing email, functions, certificate
		// renewal) will not run. Console calls the API via relative paths
		// that assume one shared origin; on its own port here, routing
		// both through one ingress host needs a manual Caddy path rule.
		Compose: `services:
  appwrite:
    image: appwrite/appwrite:1.7.4
    ports: ["80:80"]
    environment:
      _APP_ENV: production
      _APP_OPENSSL_KEY_V1: $SERVICE_HEX_64_OPENSSLKEY
      _APP_DOMAIN: ${SERVICE_FQDN_APPWRITE:-localhost}
      _APP_CONSOLE_WHITELIST_ROOT: enabled
      _APP_REDIS_HOST: redis
      _APP_REDIS_PORT: "6379"
      _APP_DB_HOST: db
      _APP_DB_PORT: "3306"
      _APP_DB_SCHEMA: appwrite
      _APP_DB_USER: $SERVICE_USER_MARIADB
      _APP_DB_PASS: $SERVICE_PASSWORD_MARIADB
    volumes:
      - appwrite_uploads:/storage/uploads
      - appwrite_cache:/storage/cache
      - appwrite_config:/storage/config
      - appwrite_certificates:/storage/certificates
    depends_on: [db, redis]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/v1/health"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 30s
  appwrite-console:
    image: appwrite/console:6.1.28
    ports: ["8081:80"]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 15s
      timeout: 5s
      retries: 5
  db:
    image: mariadb:10.11
    environment:
      MYSQL_DATABASE: appwrite
      MYSQL_USER: $SERVICE_USER_MARIADB
      MYSQL_PASSWORD: $SERVICE_PASSWORD_MARIADB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - appwrite_db_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 30s
  redis:
    image: redis:7.2.4-alpine
    volumes:
      - appwrite_redis_data:/data
`,
	},
	{ //nolint:gosec // GITEA__database__PASSWD/POSTGRES_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "gitea-postgres",
		Name:                   "Gitea (Postgres)",
		Slogan:                 "Gitea backed by Postgres instead of its default MySQL, for operators standardizing on one database engine.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.gitea.com",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag not verified against a live registry in this environment;
		// the image repository and major line are correct.
		Compose: `services:
  gitea:
    image: gitea/gitea:1.23.1
    ports: ["3000:3000", "2222:22"]
    environment:
      GITEA__database__DB_TYPE: postgres
      GITEA__database__HOST: db:5432
      GITEA__database__NAME: gitea
      GITEA__database__USER: gitea
      GITEA__database__PASSWD: $SERVICE_PASSWORD_DB
    volumes:
      - gitea_postgres_data:/data
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/healthz || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: gitea
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: gitea
    volumes:
      - gitea_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U gitea -d gitea"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // FORGEJO__security__SECRET_KEY/database__PASSWD below are compose magic-var tokens, not real credentials
		ID:                     "forgejo-postgres",
		Name:                   "Forgejo (Postgres)",
		Slogan:                 "Forgejo backed by Postgres instead of its default SQLite, for a multi-writer-safe production setup.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://forgejo.org/docs/latest/admin/installation-docker/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag unverified: codeberg.org returned 401 in this environment.
		Compose: `services:
  forgejo:
    image: codeberg.org/forgejo/forgejo:9.0.3
    ports: ["3000:3000"]
    environment:
      FORGEJO__server__ROOT_URL: ${SERVICE_FQDN_FORGEJO:-http://localhost:3000}
      FORGEJO__security__SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      FORGEJO__database__DB_TYPE: postgres
      FORGEJO__database__HOST: db:5432
      FORGEJO__database__NAME: forgejo
      FORGEJO__database__USER: forgejo
      FORGEJO__database__PASSWD: $SERVICE_PASSWORD_DB
    volumes:
      - forgejo_postgres_data:/data
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: forgejo
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: forgejo
    volumes:
      - forgejo_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U forgejo -d forgejo"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // GITLAB_ROOT_PASSWORD below is a compose magic-var token, not a real credential
		ID:                     "gitlab",
		Name:                   "GitLab",
		Slogan:                 "GitLab Community Edition: Git hosting, CI/CD, issues, and a container registry in one Omnibus image.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.gitlab.com/ee/install/docker.html",
		RecommendedMemoryBytes: 4294967296, // 4096Mi
		// Tag unverified against a live registry in this environment.
		// nginx['listen_port']/listen_https pin the bundled nginx to plain
		// HTTP on 80, matching how this platform's own ingress terminates
		// TLS in front, the same override the existing keycloak entry uses
		// (KC_PROXY_HEADERS) for an identical reason. First boot can take
		// several minutes; expect a long start_period before Ready.
		Compose: `services:
  gitlab:
    image: gitlab/gitlab-ce:17.5.2-ce.0
    ports: ["8080:80", "2224:22"]
    environment:
      GITLAB_OMNIBUS_CONFIG: "external_url 'http://localhost:8080'; nginx['listen_port'] = 80; nginx['listen_https'] = false; gitlab_rails['gitlab_shell_ssh_port'] = 2224;"
      GITLAB_ROOT_PASSWORD: $SERVICE_PASSWORD_ROOT
    volumes:
      - gitlab_config:/etc/gitlab
      - gitlab_logs:/var/log/gitlab
      - gitlab_data:/var/opt/gitlab
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/-/health"]
      interval: 30s
      timeout: 10s
      retries: 10
      start_period: 300s
`,
	},
	{ //nolint:gosec // REGISTRY_AUTH_USER/PASS below are compose magic-var tokens, not real credentials
		ID:                     "docker-registry-auth",
		Name:                   "Docker Registry (Authenticated)",
		Slogan:                 "A private container registry with HTTP basic auth baked in at boot, unlike the catalog's open registry entry.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://distribution.github.io/distribution/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// No bind-mount support for a pre-built htpasswd file, so command:
		// generates one at container start from HTPASSWD_USER/PASS: not
		// REGISTRY_*, since registry:3 auto-maps every REGISTRY_ env var
		// onto its own config and a REGISTRY_AUTH_PASS collides with that.
		Compose: `services:
  registry:
    image: registry:3
    ports: ["5000:5000"]
    environment:
      HTPASSWD_USER: $SERVICE_USER_REGISTRY
      HTPASSWD_PASS: $SERVICE_PASSWORD_REGISTRY
      REGISTRY_AUTH: htpasswd
      REGISTRY_AUTH_HTPASSWD_REALM: "Registry Realm"
      REGISTRY_AUTH_HTPASSWD_PATH: /auth/registry.htpasswd
    command: ["/bin/sh", "-c", 'apk add --no-cache apache2-utils >/dev/null 2>&1 && mkdir -p /auth && htpasswd -Bbc /auth/registry.htpasswd "$$HTPASSWD_USER" "$$HTPASSWD_PASS" && exec registry serve /etc/distribution/config.yml']
    volumes:
      - registry_auth_data:/var/lib/registry
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 5000 || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 10s
`,
	},
	{ //nolint:gosec // DATABASE_URL/DATA_ENCRYPTION_KEY below are compose magic-var tokens, not real credentials
		ID:                     "hoppscotch",
		Name:                   "Hoppscotch",
		Slogan:                 "An open-source API development platform, a self-hosted alternative to Postman, backed by Postgres.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.hoppscotch.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Upstream's own compose runs a separate one-shot "prisma migrate
		// deploy" init container before the backend starts; this platform
		// has no one-shot-container equivalent (restart: is not enforced
		// as a run-once policy), so this relies on the backend image
		// applying its own pending migrations on normal startup instead.
		Compose: `services:
  hoppscotch:
    image: hoppscotch/hoppscotch:2026.2.1
    ports: ["3080:80"]
    environment:
      VITE_ALLOWED_AUTH_PROVIDERS: EMAIL
      DATABASE_URL: postgresql://hoppscotch:$SERVICE_PASSWORD_DB@db:5432/hoppscotch
      DATA_ENCRYPTION_KEY: $SERVICE_BASE64_32_DATAENCRYPTIONKEY
      WHITELISTED_ORIGINS: ${SERVICE_FQDN_HOPPSCOTCH:-http://localhost:3080}
      VITE_BASE_URL: ${SERVICE_FQDN_HOPPSCOTCH:-http://localhost:3080}
      ENABLE_SUBPATH_BASED_ACCESS: "true"
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
  db:
    image: postgres:15-alpine
    environment:
      POSTGRES_USER: hoppscotch
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: hoppscotch
    volumes:
      - hoppscotch_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U hoppscotch -d hoppscotch"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "jupyter-notebook",
		Name:                   "Jupyter Notebook",
		Slogan:                 "A Jupyter Notebook server for interactive Python data work, protected by a generated access token.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://jupyter.org/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  jupyter:
    image: quay.io/jupyter/base-notebook:latest
    ports: ["8888:8888"]
    environment:
      JUPYTER_TOKEN: $SERVICE_PASSWORD_TOKEN
    volumes:
      - jupyter_work:/home/jovyan/work
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8888/ || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 30s
`,
	},
	{ //nolint:gosec // DATABASE_PASSWORD/JWT_SECRET/ADMIN_JWT_SECRET below are compose magic-var tokens, not real credentials
		ID:                     "strapi",
		Name:                   "Strapi",
		Slogan:                 "An open-source headless CMS with a customizable admin panel and a REST/GraphQL content API.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.strapi.io/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// No official turnkey "run anywhere" Strapi image exists (a real
		// project normally builds its own from a template); this uses the
		// same community-maintained production image most self-host
		// guides reach for, dropping the source template's dev-mode Vite
		// HMR workaround since this platform runs it as a plain
		// production service.
		Compose: `services:
  strapi:
    image: elestio/strapi-production:v5.33.4
    ports: ["1337:1337"]
    environment:
      DATABASE_CLIENT: postgres
      DATABASE_HOST: db
      DATABASE_PORT: "5432"
      DATABASE_NAME: strapi
      DATABASE_USERNAME: strapi
      DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
      JWT_SECRET: $SERVICE_BASE64_64_JWTSECRET
      ADMIN_JWT_SECRET: $SERVICE_BASE64_64_ADMINSECRET
      APP_KEYS: $SERVICE_BASE64_64_APPKEYS
      NODE_ENV: production
    volumes:
      - strapi_uploads:/opt/app/public/uploads
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:1337/_health || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: strapi
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: strapi
    volumes:
      - strapi_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U strapi -d strapi"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{ //nolint:gosec // KC_BOOTSTRAP_ADMIN_PASSWORD/KC_DB_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "keycloak-postgres",
		Name:                   "Keycloak (Postgres)",
		Slogan:                 "Keycloak backed by Postgres instead of its default embedded database, for a real multi-instance setup.",
		Category:               "Security",
		DocumentationURL:       "https://www.keycloak.org/documentation",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		// Same production-mode fixes the catalog's own plain "keycloak"
		// entry needs: KC_LEGACY_OBSERVABILITY_INTERFACE keeps
		// /health/ready on the main port instead of Keycloak 26's default
		// split to port 9000, and KC_PROXY_HEADERS trusts this platform's
		// own TLS-terminating ingress in front.
		Compose: `services:
  keycloak:
    image: quay.io/keycloak/keycloak:26.1
    command: ["start"]
    ports: ["8080:8080"]
    environment:
      KC_BOOTSTRAP_ADMIN_USERNAME: $SERVICE_USER_ADMIN
      KC_BOOTSTRAP_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      KC_DB: postgres
      KC_DB_URL: jdbc:postgresql://db:5432/keycloak
      KC_DB_USERNAME: keycloak
      KC_DB_PASSWORD: $SERVICE_PASSWORD_DB
      KC_HTTP_ENABLED: "true"
      KC_HEALTH_ENABLED: "true"
      KC_LEGACY_OBSERVABILITY_INTERFACE: "true"
      KC_HOSTNAME: ${SERVICE_FQDN_KEYCLOAK:-http://localhost:8080}
      KC_PROXY_HEADERS: xforwarded
    volumes:
      - keycloak_postgres_data:/opt/keycloak/data
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/health/ready || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 10
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: keycloak
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: keycloak
    volumes:
      - keycloak_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U keycloak -d keycloak"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "vault",
		Name:                   "Vault",
		Slogan:                 "HashiCorp Vault for secrets storage, encryption as a service, and dynamic credentials, in file-storage mode.",
		Category:               "Security",
		DocumentationURL:       "https://developer.hashicorp.com/vault/docs/deploy/run-container",
		RecommendedMemoryBytes: 268435456, // 256Mi
		// disable_mlock avoids needing the IPC_LOCK capability, which this
		// platform's compose subset has no field for anyway. The
		// healthcheck uses the vault CLI's own status check rather than
		// an HTTP probe: Vault's real /v1/sys/health semantics need query
		// parameters (standbyok, uninitcode) store.ServiceProbe has no
		// field for, and a freshly deployed Vault starts sealed by design
		// until an operator runs `vault operator init` and `unseal`, so
		// "not ready yet" here is Vault behaving correctly, not a probe bug.
		Compose: `services:
  vault:
    image: hashicorp/vault:1.18
    command: ["server"]
    ports: ["8200:8200"]
    environment:
      VAULT_ADDR: http://127.0.0.1:8200
      VAULT_LOCAL_CONFIG: '{"ui":true,"disable_mlock":true,"storage":{"file":{"path":"/vault/file"}},"listener":{"tcp":{"address":"0.0.0.0:8200","tls_disable":true}}}'
    volumes:
      - vault_data:/vault/file
    healthcheck:
      test: ["CMD", "vault", "status"]
      interval: 10s
      timeout: 5s
      retries: 12
      start_period: 15s
`,
	},
	{
		ID:                     "elasticsearch-kibana",
		Name:                   "Elasticsearch + Kibana",
		Slogan:                 "Elasticsearch paired with Kibana for log and document search with a visual dashboard.",
		Category:               "Database Tools",
		DocumentationURL:       "https://www.elastic.co/docs/deploy-manage/deploy/self-managed/install-kibana-with-docker",
		RecommendedMemoryBytes: 2147483648, // 2048Mi
		// Security is disabled on both sides rather than wired through a
		// service-account token: the source template's token-generator
		// sidecar is a one-shot container with no equivalent here (see
		// the hoppscotch entry's own comment on that limitation), so a
		// secured ES would otherwise leave Kibana permanently unable to
		// authenticate on first boot.
		Compose: `services:
  elasticsearch:
    image: elasticsearch:9.1.2
    ports: ["9200:9200"]
    environment:
      discovery.type: single-node
      xpack.security.enabled: "false"
      ES_JAVA_OPTS: "-Xms512m -Xmx512m"
    volumes:
      - es_data:/usr/share/elasticsearch/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9200/_cluster/health"]
      interval: 15s
      timeout: 10s
      retries: 20
      start_period: 60s
  kibana:
    image: kibana:9.1.2
    ports: ["5601:5601"]
    environment:
      ELASTICSEARCH_HOSTS: http://elasticsearch:9200
      SERVER_PUBLICBASEURL: ${SERVICE_FQDN_KIBANA:-http://localhost:5601}
      XPACK_ENCRYPTEDSAVEDOBJECTS_ENCRYPTIONKEY: $SERVICE_HEX_32_ENCRYPTIONKEY
    depends_on: [elasticsearch]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5601/api/status"]
      interval: 15s
      timeout: 10s
      retries: 30
      start_period: 60s
`,
	},
	{ //nolint:gosec // GF_SECURITY_ADMIN_PASSWORD/GF_DATABASE_PASSWORD below are compose magic-var tokens, not real credentials
		ID:                     "grafana-postgres",
		Name:                   "Grafana (Postgres)",
		Slogan:                 "Grafana backed by Postgres instead of its default embedded SQLite, for multi-instance or external-DB setups.",
		Category:               "Monitoring",
		DocumentationURL:       "https://grafana.com/docs/grafana/latest/",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  grafana:
    image: grafana/grafana:11.2.0
    ports: ["3000:3000"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      GF_DATABASE_TYPE: postgres
      GF_DATABASE_HOST: db:5432
      GF_DATABASE_NAME: grafana
      GF_DATABASE_USER: grafana
      GF_DATABASE_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - grafana_postgres_data:/var/lib/grafana
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/health || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 30s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: grafana
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: grafana
    volumes:
      - grafana_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U grafana -d grafana"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
}
