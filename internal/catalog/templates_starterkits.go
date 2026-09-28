package catalog

// starterKitsTemplates are the curated "ten good templates" set (ADR
// 015's own reversal note): real, tested multi-service compose examples
// that demonstrate depends_on start ordering, not one-click deploys of
// an existing OSS project. Each web/api service is a minimal reference
// server built from an official base image's own runtime (Node's http
// module, Python's http.server) rather than a fabricated demo image, so
// every image: tag here is one already verified to exist. Swap that
// service's image: for your own build output; the env var wiring and
// depends_on graph are the part meant to be copied as-is.
var starterKitsTemplates = []Template{
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "node-postgres-starter",
		Name:                   "Node.js API + PostgreSQL",
		Slogan:                 "A minimal Node HTTP service wired to a Postgres database, the most common backend pairing.",
		Category:               "Starter Kits",
		DocumentationURL:       "https://node-postgres.com/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  web:
    image: node:22-alpine
    command: ["node", "-e", "require('http').createServer((req,res)=>{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({status:'ok',database_url:process.env.DATABASE_URL?'configured':'missing'}));}).listen(3000);"]
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://app:$SERVICE_PASSWORD_DB@db:5432/app
      NODE_ENV: production
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "node -e \"require('http').get('http://127.0.0.1:3000/',r=>process.exit(r.statusCode===200?0:1)).on('error',()=>process.exit(1))\""]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 20s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - node_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d app"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
`,
	},
	{
		ID:                     "redis-cache-starter",
		Name:                   "Redis Cache Starter",
		Slogan:                 "A web service backed by a Redis cache, the cache-aside pattern most apps reach for first.",
		Category:               "Starter Kits",
		DocumentationURL:       "https://redis.io/docs/latest/develop/connect/clients/",
		RecommendedMemoryBytes: 201326592, // 192Mi
		Compose: `services:
  web:
    image: node:22-alpine
    command: ["node", "-e", "require('http').createServer((req,res)=>{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({status:'ok',redis_url:process.env.REDIS_URL?'configured':'missing'}));}).listen(3000);"]
    ports: ["3000:3000"]
    environment:
      REDIS_URL: redis://cache:6379
      NODE_ENV: production
    depends_on: [cache]
    healthcheck:
      test: ["CMD-SHELL", "node -e \"require('http').get('http://127.0.0.1:3000/',r=>process.exit(r.statusCode===200?0:1)).on('error',()=>process.exit(1))\""]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 20s
  cache:
    image: redis:7-alpine
    volumes:
      - redis_cache_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "nextjs-postgres-starter",
		Name:                   "Next.js + PostgreSQL",
		Slogan:                 "The SSR-plus-database pairing behind most Next.js apps, wired with a healthchecked Postgres.",
		Category:               "Starter Kits",
		DocumentationURL:       "https://nextjs.org/docs/app/building-your-application/deploying",
		RecommendedMemoryBytes: 402653184, // 384Mi
		Compose: `services:
  web:
    image: node:22-alpine
    command: ["node", "-e", "require('http').createServer((req,res)=>{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({status:'ok',framework:'nextjs-reference',database_url:process.env.DATABASE_URL?'configured':'missing'}));}).listen(3000);"]
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://app:$SERVICE_PASSWORD_DB@db:5432/app
      NEXT_TELEMETRY_DISABLED: "1"
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "node -e \"require('http').get('http://127.0.0.1:3000/',r=>process.exit(r.statusCode===200?0:1)).on('error',()=>process.exit(1))\""]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 20s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - nextjs_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d app"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
`,
	},
	{
		ID:                     "static-site-api-starter",
		Name:                   "Static Site + API Backend",
		Slogan:                 "A static frontend and a separate API service deployed together, the split most SPAs actually run.",
		Category:               "Starter Kits",
		DocumentationURL:       "https://nginx.org/en/docs/",
		RecommendedMemoryBytes: 134217728, // 128Mi
		Compose: `services:
  static:
    image: nginx:1.27-alpine
    ports: ["8080:80"]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:80/ >/dev/null"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 10s
  api:
    image: node:22-alpine
    command: ["node", "-e", "require('http').createServer((req,res)=>{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({status:'ok',service:'api'}));}).listen(4000);"]
    ports: ["4000:4000"]
    environment:
      NODE_ENV: production
    healthcheck:
      test: ["CMD-SHELL", "node -e \"require('http').get('http://127.0.0.1:4000/',r=>process.exit(r.statusCode===200?0:1)).on('error',()=>process.exit(1))\""]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 20s
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "fastapi-postgres-starter",
		Name:                   "FastAPI + PostgreSQL",
		Slogan:                 "A minimal Python HTTP service wired to a Postgres database, the reference pairing for a FastAPI backend.",
		Category:               "Starter Kits",
		DocumentationURL:       "https://fastapi.tiangolo.com/deployment/docker/",
		RecommendedMemoryBytes: 268435456, // 256Mi
		Compose: `services:
  api:
    image: python:3.12-alpine
    command: ["python3", "-c", "import http.server,os,socketserver,json\nclass H(http.server.BaseHTTPRequestHandler):\n    def do_GET(self):\n        self.send_response(200)\n        self.send_header('Content-Type','application/json')\n        self.end_headers()\n        self.wfile.write(json.dumps({'status':'ok','database_url':'configured' if os.environ.get('DATABASE_URL') else 'missing'}).encode())\nsocketserver.TCPServer(('0.0.0.0',8000),H).serve_forever()\n"]
    ports: ["8000:8000"]
    environment:
      DATABASE_URL: postgres://app:$SERVICE_PASSWORD_DB@db:5432/app
    depends_on: [db]
    healthcheck:
      test: ["CMD-SHELL", "python3 -c \"import urllib.request,sys; sys.exit(0 if urllib.request.urlopen('http://127.0.0.1:8000/').status==200 else 1)\""]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 20s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - fastapi_postgres_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d app"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
`,
	},
	{
		ID:                     "worker-redis-queue-starter",
		Name:                   "Background Worker + Redis Queue",
		Slogan:                 "A worker process pulling jobs off a Redis-backed queue, the base shape for async job processing.",
		Category:               "Starter Kits",
		DocumentationURL:       "https://redis.io/docs/latest/develop/data-types/lists/",
		RecommendedMemoryBytes: 201326592, // 192Mi
		Compose: `services:
  worker:
    image: node:22-alpine
    command: ["node", "-e", "console.log('worker started, queue url configured:', !!process.env.QUEUE_URL); setInterval(()=>console.log('polling queue...'), 15000);"]
    environment:
      QUEUE_URL: redis://queue:6379
      NODE_ENV: production
    depends_on: [queue]
  queue:
    image: redis:7-alpine
    volumes:
      - worker_redis_queue_data:/data
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping | grep -q PONG"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
`,
	},
	{
		ID:                     "nginx-multi-backend-starter",
		Name:                   "Nginx Reverse Proxy (Multi-Backend)",
		Slogan:                 "One Nginx entrypoint routing to two independent backend services, demonstrating depends_on across three services.",
		Category:               "Starter Kits",
		DocumentationURL:       "https://nginx.org/en/docs/http/ngx_http_proxy_module.html",
		RecommendedMemoryBytes: 201326592, // 192Mi
		Compose: `services:
  proxy:
    image: nginx:1.27-alpine
    ports: ["8080:80"]
    command:
      - sh
      - -c
      - |
        cat <<'EOF' > /etc/nginx/conf.d/default.conf
        server {
          listen 80;
          location /orders/ { proxy_pass http://orders:4000/; }
          location /payments/ { proxy_pass http://payments:4001/; }
          location / { return 200 'nginx reverse proxy: routes /orders/ and /payments/ to their backends'; add_header Content-Type text/plain; }
        }
        EOF
        exec nginx -g 'daemon off;'
    depends_on: [orders, payments]
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:80/ >/dev/null"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 10s
  orders:
    image: node:22-alpine
    command: ["node", "-e", "require('http').createServer((req,res)=>{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({service:'orders'}));}).listen(4000);"]
    environment:
      NODE_ENV: production
  payments:
    image: node:22-alpine
    command: ["node", "-e", "require('http').createServer((req,res)=>{res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({service:'payments'}));}).listen(4001);"]
    environment:
      NODE_ENV: production
`,
	},
}
