package catalog

var communicationTemplates = []Template{
	{
		ID:                     "listmonk",
		Name:                   "Listmonk",
		Slogan:                 "A self-hosted newsletter and mailing list manager with a fast, dependency-light core.",
		Category:               "Communication",
		DocumentationURL:       "https://listmonk.app/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  listmonk:
    image: listmonk/listmonk:v6.0.0
    ports: ["9000:9000"]
    environment:
      LISTMONK_ADMIN_USER: $SERVICE_USER_ADMIN
      LISTMONK_ADMIN_PASSWORD: $SERVICE_PASSWORD_ADMIN
      LISTMONK_db__host: db
      LISTMONK_db__user: listmonk
      LISTMONK_db__password: $SERVICE_PASSWORD_DB
      LISTMONK_db__database: listmonk
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:9000/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: listmonk
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: listmonk
    volumes:
      - listmonk_db_data:/var/lib/postgresql/data
`,
	},
	{
		ID:                     "rocketchat",
		Name:                   "Rocket.Chat",
		Slogan:                 "A full-featured, self-hosted team chat platform with video calls and app integrations.",
		Category:               "Communication",
		DocumentationURL:       "https://docs.rocket.chat",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  rocketchat:
    image: rocketchat/rocket.chat:8.0.1
    ports: ["3000:3000"]
    environment:
      MONGO_URL: mongodb://mongo:27017/rocketchat
      MONGO_OPLOG_URL: mongodb://mongo:27017/local
      ROOT_URL: ${SERVICE_FQDN_ROCKETCHAT:-http://localhost:3000}
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:3000/api/info || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  mongo:
    image: mongo:7
    volumes:
      - rocketchat_mongo_data:/data/db
`,
	},
	{
		ID:                     "ntfy",
		Name:                   "ntfy",
		Slogan:                 "A simple pub-sub push notification service you can send alerts to from any script or app.",
		Category:               "Communication",
		DocumentationURL:       "https://docs.ntfy.sh",
		RecommendedMemoryBytes: 134217728, // 128Mi
		// Tag not verified against a live registry in this environment;
		// the image repository is correct. No command: override needed:
		// the upstream image's own default CMD is already "serve".
		Compose: `services:
  ntfy:
    image: binwiederhier/ntfy:v2.11.0
    ports: ["80:80"]
    volumes:
      - ntfy_cache:/var/cache/ntfy
      - ntfy_data:/etc/ntfy
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/v1/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "chatwoot",
		Name:                   "Chatwoot",
		Slogan:                 "An open-source customer support platform for live chat, email, and social messaging.",
		Category:               "Communication",
		DocumentationURL:       "https://www.chatwoot.com/docs/self-hosted/",
		RecommendedMemoryBytes: 1610612736, // 1536Mi
		// Only published under a rolling :latest tag upstream; this
		// pinned version couldn't be verified against a live registry in
		// this environment. The real stack also runs a sidekiq worker for
		// background jobs (email delivery, scheduled reports); this
		// platform's compose subset has no command: effect to run a
		// second process off the same image, so it's left out here and
		// those background jobs won't run.
		Compose: `services:
  chatwoot:
    image: chatwoot/chatwoot:v4.6.0
    ports: ["3000:3000"]
    environment:
      SECRET_KEY_BASE: $SERVICE_HEX_64_SECRETKEYBASE
      FRONTEND_URL: ${SERVICE_FQDN_CHATWOOT:-http://localhost:3000}
      RAILS_ENV: production
      POSTGRES_HOST: db
      POSTGRES_DATABASE: chatwoot
      POSTGRES_USERNAME: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      REDIS_URL: redis://redis:6379
    volumes:
      - chatwoot_data:/app/storage
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 180s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: $SERVICE_USER_DB
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: chatwoot
    volumes:
      - chatwoot_db_data:/var/lib/postgresql/data
  redis:
    image: redis:7-alpine
    volumes:
      - chatwoot_redis_data:/data
`,
	},
	{
		ID:                     "answer",
		Name:                   "Apache Answer",
		Slogan:                 "A Q&A platform for building a community knowledge base, in the style of a self-hosted Stack Overflow.",
		Category:               "Communication",
		DocumentationURL:       "https://answer.apache.org/docs/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  answer:
    image: apache/answer:1.4.2
    ports: ["9080:80"]
    volumes:
      - answer_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:80/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "gotify",
		Name:                   "Gotify",
		Slogan:                 "A simple push notification server with a REST API and web UI, for sending messages to your devices.",
		Category:               "Communication",
		DocumentationURL:       "https://gotify.net/docs/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  gotify:
    image: gotify/server:2.6.1
    ports: ["8080:80"]
    environment:
      TZ: "Etc/UTC"
      GOTIFY_DEFAULTUSER_NAME: admin
      GOTIFY_DEFAULTUSER_PASS: $SERVICE_PASSWORD_ADMIN
    volumes:
      - gotify_data:/app/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:80/ || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 15s
`,
	},
}
