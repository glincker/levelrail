package catalog

var financeTemplates = []Template{
	{
		ID:                     "firefly-iii",
		Name:                   "Firefly III",
		Slogan:                 "A self-hosted personal finance manager for tracking budgets, bills, and spending.",
		Category:               "Finance",
		DocumentationURL:       "https://docs.firefly-iii.org",
		RecommendedMemoryBytes: 536870912, // 512Mi
		// Tag not verified against a live registry in this environment;
		// the image repository and major line are correct.
		Compose: `services:
  firefly:
    image: fireflyiii/core:version-6.2.3
    ports: ["8080:8080"]
    environment:
      APP_KEY: $SERVICE_BASE64_32_APPKEY
      DB_CONNECTION: mysql
      DB_HOST: db
      DB_DATABASE: firefly
      DB_USERNAME: firefly
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      APP_URL: ${SERVICE_FQDN_FIREFLY:-http://localhost:8080}
    volumes:
      - firefly_upload_data:/var/www/html/storage/upload
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 90s
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: firefly
      MYSQL_USER: firefly
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - firefly_db_data:/var/lib/mysql
`,
	},
	{
		ID:                     "invoice-ninja",
		Name:                   "Invoice Ninja",
		Slogan:                 "Self-hosted invoicing, quotes, and payments for freelancers and small businesses.",
		Category:               "Finance",
		DocumentationURL:       "https://invoiceninja.github.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  invoiceninja:
    image: invoiceninja/invoiceninja:5
    ports: ["8080:80"]
    environment:
      APP_URL: ${SERVICE_FQDN_INVOICENINJA:-http://localhost:8080}
      APP_KEY: $SERVICE_BASE64_32_APPKEY
      DB_HOST: db
      DB_DATABASE: invoiceninja
      DB_USERNAME: invoiceninja
      DB_PASSWORD: $SERVICE_PASSWORD_DB
      REDIS_HOST: redis
    volumes:
      - invoiceninja_data:/var/www/app/storage
    healthcheck:
      test: ["CMD-SHELL", "sh -c ': < /dev/tcp/127.0.0.1/80' || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  db:
    image: mariadb:11
    environment:
      MYSQL_DATABASE: invoiceninja
      MYSQL_USER: invoiceninja
      MYSQL_PASSWORD: $SERVICE_PASSWORD_DB
      MYSQL_ROOT_PASSWORD: $SERVICE_PASSWORD_MARIADBROOT
    volumes:
      - invoiceninja_db_data:/var/lib/mysql
  redis:
    image: redis:7.4-alpine
    volumes:
      - invoiceninja_redis_data:/data
`,
	},
	{
		ID:                     "actual-budget",
		Name:                   "Actual Budget",
		Slogan:                 "A local-first, envelope-style budgeting app with optional multi-device sync.",
		Category:               "Finance",
		DocumentationURL:       "https://actualbudget.org/docs/install/docker/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  actual:
    image: actualbudget/actual-server:26.9.0
    ports: ["5006:5006"]
    volumes:
      - actual_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:5006/health"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
}
