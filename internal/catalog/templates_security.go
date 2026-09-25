package catalog

var securityTemplates = []Template{
	{
		ID:                     "vaultwarden",
		Name:                   "Vaultwarden",
		Slogan:                 "A lightweight, self-hosted password manager server compatible with the Bitwarden clients.",
		Category:               "Security",
		DocumentationURL:       "https://github.com/dani-garcia/vaultwarden/wiki",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  vaultwarden:
    image: vaultwarden/server:1.32.1
    ports: ["8080:80"]
    environment:
      ADMIN_TOKEN: $SERVICE_HEX_64_ADMINTOKEN
    volumes:
      - vaultwarden_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/alive || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "passbolt",
		Name:                   "Passbolt",
		Slogan:                 "An open-source password manager built for teams, compatible with the usual browser extensions.",
		Category:               "Security",
		DocumentationURL:       "https://www.passbolt.com/docs",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Only published under a rolling :latest-ce tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  passbolt:
    image: passbolt/passbolt:4.12.0-ce
    ports: ["80:80"]
    environment:
      APP_FULL_BASE_URL: ${SERVICE_FQDN_PASSBOLT:-http://localhost}
      PASSBOLT_SSL_FORCE: "false"
      DATASOURCES_DEFAULT_HOST: db
      DATASOURCES_DEFAULT_USERNAME: passbolt
      DATASOURCES_DEFAULT_PASSWORD: $SERVICE_PASSWORD_DB
      DATASOURCES_DEFAULT_DATABASE: passbolt
    volumes:
      - passbolt_gpg:/etc/passbolt/gpg
      - passbolt_jwt:/etc/passbolt/jwt
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/healthcheck/status.json"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
  db:
    image: mariadb:11
    environment:
      MARIADB_ROOT_PASSWORD: $SERVICE_PASSWORD_DBROOT
      MARIADB_DATABASE: passbolt
      MARIADB_USER: passbolt
      MARIADB_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - passbolt_db_data:/var/lib/mysql
`,
	},
	{
		ID:                     "keycloak",
		Name:                   "Keycloak",
		Slogan:                 "An open-source identity and access management server with SSO, OAuth2, and SAML support.",
		Category:               "Security",
		DocumentationURL:       "https://www.keycloak.org/documentation",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  keycloak:
    image: quay.io/keycloak/keycloak:26.1
    command: ["start"]
    ports: ["8080:8080"]
    environment:
      KC_BOOTSTRAP_ADMIN_USERNAME: $SERVICE_USER_ADMIN
      KC_BOOTSTRAP_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      KC_HTTP_ENABLED: "true"
      KC_HEALTH_ENABLED: "true"
      # Keycloak 26 moves /health/ready to a separate management port
      # (9000) in start as well as start-dev; this keeps it on the main
      # port so this platform's single-port probe can still reach it.
      KC_LEGACY_OBSERVABILITY_INTERFACE: "true"
      KC_HOSTNAME: ${SERVICE_FQDN_KEYCLOAK:-http://localhost:8080}
      # Production mode (start) requires a configured hostname and
      # rejects its own TLS assumptions; this platform's Caddy ingress
      # terminates TLS in front, so Keycloak trusts its proxy headers.
      KC_PROXY_HEADERS: xforwarded
    volumes:
      - keycloak_data:/opt/keycloak/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/health/ready || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "pocket-id",
		Name:                   "Pocket ID",
		Slogan:                 "A simple, secure OIDC provider that authenticates with passkeys instead of passwords.",
		Category:               "Security",
		DocumentationURL:       "https://pocket-id.org/docs/setup/installation",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  pocket-id:
    image: ghcr.io/pocket-id/pocket-id:v1.13
    ports: ["1411:1411"]
    environment:
      APP_URL: ${SERVICE_FQDN_POCKETID:-http://localhost:1411}
      TRUST_PROXY: "true"
    volumes:
      - pocket_id_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:1411/healthz"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "privatebin",
		Name:                   "PrivateBin",
		Slogan:                 "A minimalist, encrypted pastebin where the server has zero knowledge of what you paste.",
		Category:               "Security",
		DocumentationURL:       "https://github.com/PrivateBin/PrivateBin/blob/master/doc/README.md",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment.
		Compose: `services:
  privatebin:
    image: privatebin/nginx-fpm-alpine:1.7.4
    ports: ["8080:8080"]
    volumes:
      - privatebin_data:/srv/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
}
