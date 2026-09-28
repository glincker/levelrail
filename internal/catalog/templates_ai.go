package catalog

var aiTemplates = []Template{
	{
		ID:                     "open-webui",
		Name:                   "Open WebUI",
		Slogan:                 "A self-hosted chat interface for running local large language models through Ollama.",
		Category:               "AI",
		DocumentationURL:       "https://docs.openwebui.com",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  ollama:
    image: ollama/ollama:0.33.3
    volumes:
      - ollama_data:/root/.ollama
  open-webui:
    image: ghcr.io/open-webui/open-webui:0.11.3
    ports: ["8080:8080"]
    environment:
      OLLAMA_BASE_URL: http://ollama:11434
      WEBUI_SECRET_KEY: $SERVICE_HEX_32_SECRETKEY
    volumes:
      - openwebui_data:/app/backend/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8080/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{ //nolint:gosec // DATABASE_URL below is a compose magic-var token ($SERVICE_PASSWORD_DB), not a real credential
		ID:                     "linkwarden",
		Name:                   "Linkwarden",
		Slogan:                 "A bookmark manager that archives full page snapshots, not just links, so pages stay readable.",
		Category:               "Productivity",
		DocumentationURL:       "https://docs.linkwarden.app/self-hosting/setup",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  linkwarden:
    image: ghcr.io/linkwarden/linkwarden:v2.9.3
    ports: ["3000:3000"]
    environment:
      NEXTAUTH_URL: ${SERVICE_FQDN_LINKWARDEN:-http://localhost:3000}/api/v1/auth
      NEXTAUTH_SECRET: $SERVICE_HEX_32_NEXTAUTHSECRET
      DATABASE_URL: postgresql://postgres:$SERVICE_PASSWORD_DB@db:5432/linkwarden
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MEILIKEY
      MEILI_HOST: http://meilisearch:7700
    volumes:
      - linkwarden_data:/data/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 120s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: linkwarden
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
    volumes:
      - linkwarden_db_data:/var/lib/postgresql/data
  meilisearch:
    image: getmeili/meilisearch:v1.11.1
    environment:
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MEILIKEY
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - linkwarden_meili_data:/meili_data
`,
	},
	{
		ID:                     "ollama",
		Name:                   "Ollama",
		Slogan:                 "A local LLM runtime with an HTTP API. Pull any model yourself once it's running.",
		Category:               "AI",
		DocumentationURL:       "https://github.com/ollama/ollama",
		RecommendedMemoryBytes: 1073741824, // 1024Mi
		Compose: `services:
  ollama:
    image: ollama/ollama:0.33.3
    ports: ["11434:11434"]
    volumes:
      - ollama_data:/root/.ollama
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:11434/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	// The five ollama-* entries pin an explicit parameter-size tag, not
	// a bare alias (Ollama's default can change under a bare alias).
	// RecommendedMemoryBytes is Q4 quantization's rule of thumb: ~0.6 GB
	// per billion parameters plus ~1.5 GB overhead, rounded up to a
	// whole GiB. The auto-pull command polls "ollama list" for
	// readiness instead of a fixed sleep.
	{
		ID:                     "ollama-mistral",
		Name:                   "Ollama: Mistral 7B",
		Slogan:                 "Mistral's 7B instruct model, served locally through Ollama's HTTP API.",
		Category:               "AI",
		DocumentationURL:       "https://ollama.com/library/mistral",
		RecommendedMemoryBytes: 6 * 1024 * 1024 * 1024, // 7B * 0.6 GB/B + 1.5 GB overhead ~= 5.7 GB
		Compose: `services:
  ollama:
    image: ollama/ollama:0.33.3
    ports: ["11434:11434"]
    volumes:
      - ollama_data:/root/.ollama
    command: "ollama serve & until ollama list >/dev/null 2>&1; do sleep 1; done; ollama pull mistral:7b-instruct-v0.3; wait"
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:11434/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "ollama-llama3",
		Name:                   "Ollama: Llama 3 8B",
		Slogan:                 "Meta's Llama 3 8B model, served locally through Ollama's HTTP API.",
		Category:               "AI",
		DocumentationURL:       "https://ollama.com/library/llama3",
		RecommendedMemoryBytes: 7 * 1024 * 1024 * 1024, // 8B * 0.6 GB/B + 1.5 GB overhead ~= 6.3 GB
		Compose: `services:
  ollama:
    image: ollama/ollama:0.33.3
    ports: ["11434:11434"]
    volumes:
      - ollama_data:/root/.ollama
    command: "ollama serve & until ollama list >/dev/null 2>&1; do sleep 1; done; ollama pull llama3:8b; wait"
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:11434/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "ollama-qwen",
		Name:                   "Ollama: Qwen 2.5 7B",
		Slogan:                 "Alibaba's Qwen 2.5 7B model, served locally through Ollama's HTTP API.",
		Category:               "AI",
		DocumentationURL:       "https://ollama.com/library/qwen2.5",
		RecommendedMemoryBytes: 6 * 1024 * 1024 * 1024, // 7B * 0.6 GB/B + 1.5 GB overhead ~= 5.7 GB
		Compose: `services:
  ollama:
    image: ollama/ollama:0.33.3
    ports: ["11434:11434"]
    volumes:
      - ollama_data:/root/.ollama
    command: "ollama serve & until ollama list >/dev/null 2>&1; do sleep 1; done; ollama pull qwen2.5:7b; wait"
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:11434/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "ollama-phi",
		Name:                   "Ollama: Phi-3 Mini",
		Slogan:                 "Microsoft's Phi-3 Mini (3.8B), the smallest model in this catalog, good for constrained nodes.",
		Category:               "AI",
		DocumentationURL:       "https://ollama.com/library/phi3",
		RecommendedMemoryBytes: 4 * 1024 * 1024 * 1024, // 3.8B * 0.6 GB/B + 1.5 GB overhead ~= 3.8 GB
		Compose: `services:
  ollama:
    image: ollama/ollama:0.33.3
    ports: ["11434:11434"]
    volumes:
      - ollama_data:/root/.ollama
    command: "ollama serve & until ollama list >/dev/null 2>&1; do sleep 1; done; ollama pull phi3:3.8b; wait"
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:11434/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
	{
		ID:                     "ollama-deepseek",
		Name:                   "Ollama: DeepSeek-R1 7B",
		Slogan:                 "DeepSeek's R1 distilled 7B reasoning model, served locally through Ollama's HTTP API.",
		Category:               "AI",
		DocumentationURL:       "https://ollama.com/library/deepseek-r1",
		RecommendedMemoryBytes: 6 * 1024 * 1024 * 1024, // 7B * 0.6 GB/B + 1.5 GB overhead ~= 5.7 GB
		Compose: `services:
  ollama:
    image: ollama/ollama:0.33.3
    ports: ["11434:11434"]
    volumes:
      - ollama_data:/root/.ollama
    command: "ollama serve & until ollama list >/dev/null 2>&1; do sleep 1; done; ollama pull deepseek-r1:7b; wait"
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:11434/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
`,
	},
}
