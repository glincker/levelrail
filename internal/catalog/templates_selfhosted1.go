package catalog

var selfhosted1Templates = []Template{
	{
		ID:                     "loki",
		Name:                   "Grafana Loki",
		Slogan:                 "A horizontally scalable log aggregation system that indexes labels, not full text, to keep storage cheap.",
		Category:               "Monitoring",
		DocumentationURL:       "https://grafana.com/docs/loki/latest",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  loki:
    image: grafana/loki:3.7.8
    command: ["-config.file=/etc/loki/local-config.yaml"]
    ports: ["3100:3100"]
    volumes:
      - loki_data:/loki
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3100/ready || wget -q -O /dev/null http://127.0.0.1:3100/ready || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "victoriametrics",
		Name:                   "VictoriaMetrics",
		Slogan:                 "A fast, resource-frugal time-series database that speaks the Prometheus query and remote-write APIs.",
		Category:               "Monitoring",
		DocumentationURL:       "https://docs.victoriametrics.com",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  victoriametrics:
    image: victoriametrics/victoria-metrics:v1.152.0
    command: ["-storageDataPath=/storage", "-retentionPeriod=12"]
    ports: ["8428:8428"]
    volumes:
      - victoriametrics_data:/storage
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8428/health || wget -q -O /dev/null http://127.0.0.1:8428/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "netdata",
		Name:                   "Netdata",
		Slogan:                 "Per-second infrastructure metrics with zero configuration and a live dashboard out of the box.",
		Category:               "Monitoring",
		DocumentationURL:       "https://learn.netdata.cloud",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  netdata:
    image: netdata/netdata:v2.11.1
    ports: ["19999:19999"]
    volumes:
      - netdata_config:/etc/netdata
      - netdata_lib:/var/lib/netdata
      - netdata_cache:/var/cache/netdata
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:19999/api/v1/info || wget -q -O /dev/null http://127.0.0.1:19999/api/v1/info || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "authentik",
		Name:                   "authentik",
		Slogan:                 "An identity provider and SSO platform with flows, policies, and support for OIDC, SAML, LDAP, and proxy auth.",
		Category:               "Security",
		DocumentationURL:       "https://docs.goauthentik.io",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  authentik:
    image: ghcr.io/goauthentik/server:2026.8.3
    command: ["server"]
    ports: ["9000:9000"]
    environment:
      AUTHENTIK_SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      AUTHENTIK_POSTGRESQL__HOST: db
      AUTHENTIK_POSTGRESQL__NAME: authentik
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: $SERVICE_PASSWORD_DB
      AUTHENTIK_BOOTSTRAP_PASSWORD: $SERVICE_PASSWORD_ADMIN
      AUTHENTIK_BOOTSTRAP_EMAIL: admin@example.com
    volumes:
      - authentik_media:/media
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9000/-/health/live/ || wget -q -O /dev/null http://127.0.0.1:9000/-/health/live/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  worker:
    image: ghcr.io/goauthentik/server:2026.8.3
    command: ["worker"]
    environment:
      AUTHENTIK_SECRET_KEY: $SERVICE_HEX_64_SECRETKEY
      AUTHENTIK_POSTGRESQL__HOST: db
      AUTHENTIK_POSTGRESQL__NAME: authentik
      AUTHENTIK_POSTGRESQL__USER: authentik
      AUTHENTIK_POSTGRESQL__PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - authentik_media:/media
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: authentik
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: authentik
    volumes:
      - authentik_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U authentik -d authentik"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{ //nolint:gosec // credential in URL below is a compose magic-var token, not a real credential
		ID:                     "logto",
		Name:                   "Logto",
		Slogan:                 "Modern, developer-first customer identity: sign-in experiences, social login, and OIDC without the plumbing.",
		Category:               "Security",
		DocumentationURL:       "https://docs.logto.io",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  logto:
    image: svhd/logto:1.43.0
    command: ["sh", "-c", "npm run cli db seed -- --swe && npm start"]
    ports: ["3001:3001", "3002:3002"]
    environment:
      TRUST_PROXY_HEADER: "1"
      DB_URL: postgres://logto:$SERVICE_PASSWORD_DB@db:5432/logto
      ENDPOINT: ${SERVICE_FQDN_LOGTO:-http://localhost:3001}
      ADMIN_ENDPOINT: ${SERVICE_FQDN_LOGTO_ADMIN:-http://localhost:3002}
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3001/api/status || wget -q -O /dev/null http://127.0.0.1:3001/api/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 90s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: logto
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: logto
    volumes:
      - logto_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U logto -d logto"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "lldap",
		Name:                   "lldap",
		Slogan:                 "A lightweight LDAP server with a friendly web UI, built for homelab and small-team authentication.",
		Category:               "Security",
		DocumentationURL:       "https://github.com/lldap/lldap",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  lldap:
    image: lldap/lldap:v0.6.3
    ports: ["17170:17170", "3890:3890"]
    environment:
      LLDAP_JWT_SECRET: $SERVICE_HEX_64_JWTSECRET
      LLDAP_KEY_SEED: $SERVICE_HEX_64_KEYSEED
      LLDAP_LDAP_USER_PASS: $SERVICE_PASSWORD_ADMIN
      LLDAP_LDAP_BASE_DN: dc=example,dc=com
    volumes:
      - lldap_data:/data
    healthcheck:
      test: ["CMD", "/app/lldap", "healthcheck", "--config-file", "/data/lldap_config.toml"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
	{
		ID:                     "2fauth",
		Name:                   "2FAuth",
		Slogan:                 "A web app to manage your two-factor authentication accounts and generate one-time codes.",
		Category:               "Security",
		DocumentationURL:       "https://docs.2fauth.app",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  2fauth:
    image: 2fauth/2fauth:8.0.2
    ports: ["8000:8000"]
    environment:
      APP_KEY: base64:$SERVICE_REALBASE64_32_APPKEY
      APP_URL: ${SERVICE_FQDN_2FAUTH:-http://localhost:8000}
      SITE_OWNER: admin@example.com
      DB_CONNECTION: sqlite
    volumes:
      - twofauth_data:/2fauth
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/ || wget -q -O /dev/null http://127.0.0.1:8000/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "seaweedfs",
		Name:                   "SeaweedFS",
		Slogan:                 "A distributed file and object store with an S3 gateway, built for billions of small files.",
		Category:               "Storage",
		DocumentationURL:       "https://github.com/seaweedfs/seaweedfs/wiki",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  seaweedfs:
    image: chrislusf/seaweedfs:3.99
    command: ["server", "-dir=/data", "-filer", "-s3", "-master.volumeSizeLimitMB=1024"]
    ports: ["9333:9333", "8888:8888", "8333:8333"]
    volumes:
      - seaweedfs_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9333/cluster/status || wget -q -O /dev/null http://127.0.0.1:9333/cluster/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "copyparty",
		Name:                   "copyparty",
		Slogan:                 "A portable file server with resumable uploads, a media player, and search, in a single small image.",
		Category:               "Storage",
		DocumentationURL:       "https://github.com/9001/copyparty",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  copyparty:
    image: copyparty/ac:1.20.24
    command: ["-a", "admin:$SERVICE_PASSWORD_ADMIN", "-v", "/w::rw,admin"]
    ports: ["3923:3923"]
    volumes:
      - copyparty_files:/w
      - copyparty_config:/cfg
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://127.0.0.1:3923/?reset"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "woodpecker",
		Name:                   "Woodpecker CI",
		Slogan:                 "A simple, container-native CI engine with a server and an agent, driven by pipelines in your repo.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://woodpecker-ci.org/docs",
		RecommendedMemoryBytes: 536870912,
		// WOODPECKER_GITEA_* values are placeholders to edit before deploying: a forge must be configured for login.
		Compose: `services:
  woodpecker-server:
    image: woodpeckerci/woodpecker-server:v3.18.1
    ports: ["8000:8000"]
    environment:
      WOODPECKER_HOST: ${SERVICE_FQDN_WOODPECKER_SERVER:-http://localhost:8000}
      WOODPECKER_AGENT_SECRET: $SERVICE_HEX_32_AGENTSECRET
      WOODPECKER_OPEN: "false"
      WOODPECKER_ADMIN: admin
      WOODPECKER_GITEA: "true"
      WOODPECKER_GITEA_URL: https://gitea.example.com
      WOODPECKER_GITEA_CLIENT: change-me
      WOODPECKER_GITEA_SECRET: change-me
    volumes:
      - woodpecker_data:/var/lib/woodpecker
    healthcheck:
      test: ["CMD", "/bin/woodpecker-server", "ping"]
      interval: 30s
      timeout: 5s
      retries: 3
  woodpecker-agent:
    image: woodpeckerci/woodpecker-agent:v3.18.1
    environment:
      WOODPECKER_SERVER: woodpecker-server:9000
      WOODPECKER_AGENT_SECRET: $SERVICE_HEX_32_AGENTSECRET
`,
	},
	{
		ID:                     "sonarqube",
		Name:                   "SonarQube Community",
		Slogan:                 "Continuous code quality and security analysis across 30+ languages, with quality gates for every pull request.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.sonarsource.com/sonarqube-community-build",
		RecommendedMemoryBytes: 3221225472,
		// Rolling community tag: SonarQube publishes only community, lts, and dated release tags in this form.
		Compose: `services:
  sonarqube:
    image: sonarqube:community
    ports: ["9000:9000"]
    environment:
      SONAR_JDBC_URL: jdbc:postgresql://db:5432/sonar
      SONAR_JDBC_USERNAME: sonar
      SONAR_JDBC_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - sonarqube_data:/opt/sonarqube/data
      - sonarqube_extensions:/opt/sonarqube/extensions
      - sonarqube_logs:/opt/sonarqube/logs
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9000/api/system/status || wget -q -O /dev/null http://127.0.0.1:9000/api/system/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 120s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: sonar
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: sonar
    volumes:
      - sonarqube_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U sonar -d sonar"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "jenkins",
		Name:                   "Jenkins",
		Slogan:                 "The long-running automation server for building, testing, and deploying with thousands of plugins.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://www.jenkins.io/doc",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  jenkins:
    image: jenkins/jenkins:2.583
    ports: ["8080:8080", "50000:50000"]
    volumes:
      - jenkins_home:/var/jenkins_home
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/login || wget -q -O /dev/null http://127.0.0.1:8080/login || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 90s
`,
	},
	{
		ID:                     "nexus",
		Name:                   "Sonatype Nexus Repository",
		Slogan:                 "A universal artifact repository for Maven, npm, Docker, PyPI, and more, with proxying and caching.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://help.sonatype.com/en/sonatype-nexus-repository.html",
		RecommendedMemoryBytes: 3221225472,
		Compose: `services:
  nexus:
    image: sonatype/nexus3:3.96.3
    ports: ["8081:8081"]
    volumes:
      - nexus_data:/nexus-data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8081/service/rest/v1/status || wget -q -O /dev/null http://127.0.0.1:8081/service/rest/v1/status || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 180s
`,
	},
	{
		ID:                     "semaphore",
		Name:                   "Semaphore UI",
		Slogan:                 "A modern web UI for running Ansible, Terraform, and shell tasks with schedules, history, and access control.",
		Category:               "Developer Tools",
		DocumentationURL:       "https://docs.semaphoreui.com",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  semaphore:
    image: semaphoreui/semaphore:v2.19.14
    ports: ["3000:3000"]
    environment:
      SEMAPHORE_DB_DIALECT: bolt
      SEMAPHORE_ADMIN: admin
      SEMAPHORE_ADMIN_NAME: Admin
      SEMAPHORE_ADMIN_EMAIL: admin@example.com
      SEMAPHORE_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      SEMAPHORE_ACCESS_KEY_ENCRYPTION: $SERVICE_REALBASE64_32_ENCRYPTION
    volumes:
      - semaphore_data:/var/lib/semaphore
      - semaphore_config:/etc/semaphore
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api/ping || wget -q -O /dev/null http://127.0.0.1:3000/api/ping || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
}
