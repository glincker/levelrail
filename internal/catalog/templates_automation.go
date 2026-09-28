package catalog

var automationTemplates = []Template{
	{
		ID:                     "n8n",
		Name:                   "n8n",
		Slogan:                 "Build automations and connect your tools with a visual, node-based workflow editor.",
		Category:               "Automation",
		DocumentationURL:       "https://docs.n8n.io",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  n8n:
    image: n8nio/n8n:1.62.1
    ports: ["5678:5678"]
    environment:
      N8N_ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
      N8N_HOST: "0.0.0.0"
      N8N_PORT: "5678"
    volumes:
      - n8n_data:/home/node/.n8n
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:5678/healthz || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "activepieces",
		Name:                   "Activepieces",
		Slogan:                 "An open-source, no-code automation tool for connecting apps and building AI-powered workflows.",
		Category:               "Automation",
		DocumentationURL:       "https://www.activepieces.com/docs",
		RecommendedMemoryBytes: 536870912, // 512Mi
		Compose: `services:
  activepieces:
    image: ghcr.io/activepieces/activepieces:0.75.0
    ports: ["8080:80"]
    environment:
      AP_ENCRYPTION_KEY: $SERVICE_HEX_32_ENCRYPTIONKEY
      AP_JWT_SECRET: $SERVICE_HEX_32_JWTSECRET
      AP_FRONTEND_URL: ${SERVICE_FQDN_ACTIVEPIECES:-http://localhost:8080}
      AP_POSTGRES_HOST: db
      AP_POSTGRES_DATABASE: activepieces
      AP_POSTGRES_USERNAME: activepieces
      AP_POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      AP_REDIS_HOST: redis
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: activepieces
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: activepieces
    volumes:
      - activepieces_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - activepieces_redis_data:/data
`,
	},
	{
		ID:                     "node-red",
		Name:                   "Node-RED",
		Slogan:                 "A flow-based visual editor for wiring together hardware, APIs, and online services.",
		Category:               "Automation",
		DocumentationURL:       "https://nodered.org/docs/getting-started/docker",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  nodered:
    image: nodered/node-red:4.1.15-22
    ports: ["1880:1880"]
    volumes:
      - nodered_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:1880/"]
      interval: 10s
      timeout: 5s
      retries: 3
`,
	},
}
