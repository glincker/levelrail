package catalog

var selfhosted3Templates = []Template{
	{
		ID:                     "roundcube",
		Name:                   "Roundcube Webmail",
		Slogan:                 "A browser-based IMAP email client with a clean interface, address books, and plugin support.",
		Category:               "Communication",
		DocumentationURL:       "https://roundcube.net/about",
		RecommendedMemoryBytes: 268435456,
		// ROUNDCUBEMAIL_DEFAULT_HOST and SMTP_SERVER are placeholders to edit before deploying.
		Compose: `services:
  roundcube:
    image: roundcube/roundcubemail:1.7.4-apache
    ports: ["8080:80"]
    environment:
      ROUNDCUBEMAIL_DB_TYPE: sqlite
      ROUNDCUBEMAIL_DEFAULT_HOST: ssl://mail.example.com
      ROUNDCUBEMAIL_SMTP_SERVER: tls://mail.example.com
    volumes:
      - roundcube_db:/var/roundcube/db
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "wallos",
		Name:                   "Wallos",
		Slogan:                 "A personal subscription tracker that shows what you pay each month, with reminders and multi-currency support.",
		Category:               "Finance",
		DocumentationURL:       "https://github.com/ellite/Wallos",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  wallos:
    image: bellamy/wallos:5.8.1
    ports: ["8282:80"]
    volumes:
      - wallos_db:/var/www/html/db
      - wallos_logos:/var/www/html/images/uploads/logos
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/ || wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{ //nolint:gosec // credential in URL below is a compose magic-var token, not a real credential
		ID:                     "ghostfolio",
		Name:                   "Ghostfolio",
		Slogan:                 "A privacy-first wealth tracker for stocks, ETFs, and crypto, with portfolio analytics and no ads.",
		Category:               "Finance",
		DocumentationURL:       "https://github.com/ghostfolio/ghostfolio",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  ghostfolio:
    image: ghostfolio/ghostfolio:3.72.0
    ports: ["3333:3333"]
    environment:
      DATABASE_URL: postgresql://ghostfolio:$SERVICE_PASSWORD_DB@db:5432/ghostfolio?connect_timeout=300&sslmode=prefer
      REDIS_HOST: redis
      REDIS_PORT: "6379"
      ACCESS_TOKEN_SALT: $SERVICE_HEX_32_ACCESSSALT
      JWT_SECRET_KEY: $SERVICE_HEX_32_JWT
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3333/api/v1/health || wget -q -O /dev/null http://127.0.0.1:3333/api/v1/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ghostfolio
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: ghostfolio
    volumes:
      - ghostfolio_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ghostfolio -d ghostfolio"]
      interval: 10s
      timeout: 5s
      retries: 5
  redis:
    image: redis:7-alpine
    volumes:
      - ghostfolio_redis_data:/data
`,
	},
	{
		ID:                     "silverbullet",
		Name:                   "SilverBullet",
		Slogan:                 "A programmable, Markdown-based notes app you can extend with your own scripts and queries.",
		Category:               "Productivity",
		DocumentationURL:       "https://silverbullet.md",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  silverbullet:
    image: ghcr.io/silverbulletmd/silverbullet:2.11.1
    ports: ["3000:3000"]
    environment:
      SB_USER: admin:$SERVICE_PASSWORD_ADMIN
    volumes:
      - silverbullet_space:/space
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/.ping || wget -q -O /dev/null http://127.0.0.1:3000/.ping || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
}
