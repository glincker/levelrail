package catalog

var aiml1Templates = []Template{
	{
		ID:                     "vllm",
		Name:                   "vLLM",
		Slogan:                 "A fast, high-throughput inference server that exposes an OpenAI-compatible API for open-weight language models.",
		Category:               "AI",
		DocumentationURL:       "https://docs.vllm.ai",
		RecommendedMemoryBytes: 8589934592,
		RequiresGPU:            true,
		Compose: `services:
  vllm:
    image: vllm/vllm-openai:v0.30.0
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    command: ["--model", "Qwen/Qwen2.5-0.5B-Instruct", "--host", "0.0.0.0", "--port", "8000"]
    ports: ["8000:8000"]
    environment:
      HF_HOME: /root/.cache/huggingface
    volumes:
      - vllm_hf_cache:/root/.cache/huggingface
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/health || wget -q -O /dev/null http://127.0.0.1:8000/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 300s
`,
	},
	{
		ID:                     "llama-cpp",
		Name:                   "llama.cpp Server",
		Slogan:                 "The llama.cpp HTTP server: run quantized GGUF models on plain CPUs with an OpenAI-compatible API.",
		Category:               "AI",
		DocumentationURL:       "https://github.com/ggml-org/llama.cpp/tree/master/tools/server",
		RecommendedMemoryBytes: 4294967296,
		// Rolling server tag: upstream publishes only build-numbered and rolling tags.
		Compose: `services:
  llama-cpp:
    image: ghcr.io/ggml-org/llama.cpp:server
    command: ["-hf", "ggml-org/gemma-3-1b-it-GGUF", "--host", "0.0.0.0", "--port", "8080"]
    ports: ["8080:8080"]
    environment:
      LLAMA_CACHE: /models
    volumes:
      - llama_cpp_models:/models
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/health || wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 120s
`,
	},
	{
		ID:                     "text-generation-inference",
		Name:                   "Text Generation Inference",
		Slogan:                 "Hugging Face's production inference server for transformer language models, with continuous batching and streaming.",
		Category:               "AI",
		DocumentationURL:       "https://huggingface.co/docs/text-generation-inference",
		RecommendedMemoryBytes: 8589934592,
		RequiresGPU:            true,
		Compose: `services:
  tgi:
    image: ghcr.io/huggingface/text-generation-inference:3.3.6
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    command: ["--model-id", "HuggingFaceTB/SmolLM2-360M-Instruct", "--port", "80"]
    ports: ["8080:80"]
    volumes:
      - tgi_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/health || wget -q -O /dev/null http://127.0.0.1:80/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 300s
`,
	},
	{
		ID:                     "text-embeddings-inference",
		Name:                   "Text Embeddings Inference",
		Slogan:                 "A lean server that turns text into embeddings with popular open models, on CPU with no extra setup.",
		Category:               "AI",
		DocumentationURL:       "https://huggingface.co/docs/text-embeddings-inference",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  tei:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.8
    command: ["--model-id", "BAAI/bge-small-en-v1.5", "--port", "80"]
    ports: ["8080:80"]
    volumes:
      - tei_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:80/health || wget -q -O /dev/null http://127.0.0.1:80/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 120s
`,
	},
	{
		ID:                     "comfyui",
		Name:                   "ComfyUI",
		Slogan:                 "A node-graph editor for building and running Stable Diffusion and other image generation pipelines.",
		Category:               "AI",
		DocumentationURL:       "https://docs.comfy.org",
		RecommendedMemoryBytes: 8589934592,
		RequiresGPU:            true,
		Compose: `services:
  comfyui:
    image: ghcr.io/ai-dock/comfyui:latest-cuda
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    ports: ["8188:8188"]
    environment:
      WEB_ENABLE_AUTH: "true"
      WEB_USER: admin
      WEB_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - comfyui_workspace:/workspace
    healthcheck:
      test: ["CMD-SHELL", "curl -s -o /dev/null http://127.0.0.1:8188/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 5
      start_period: 120s
`,
	},
	{
		ID:                     "stable-diffusion-webui",
		Name:                   "Stable Diffusion WebUI",
		Slogan:                 "A browser interface for generating and editing images with Stable Diffusion checkpoints.",
		Category:               "AI",
		DocumentationURL:       "https://github.com/AUTOMATIC1111/stable-diffusion-webui/wiki",
		RecommendedMemoryBytes: 8589934592,
		RequiresGPU:            true,
		Compose: `services:
  sd-webui:
    image: ghcr.io/ai-dock/stable-diffusion-webui:latest-cuda
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    ports: ["7860:7860"]
    environment:
      WEB_ENABLE_AUTH: "true"
      WEB_USER: admin
      WEB_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - sd_webui_workspace:/workspace
    healthcheck:
      test: ["CMD-SHELL", "curl -s -o /dev/null http://127.0.0.1:7860/ || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 5
      start_period: 180s
`,
	},
	{
		ID:                     "localai",
		Name:                   "LocalAI",
		Slogan:                 "A drop-in OpenAI-compatible API that runs language, image, and speech models on your own hardware.",
		Category:               "AI",
		DocumentationURL:       "https://localai.io",
		RecommendedMemoryBytes: 4294967296,
		Compose: `services:
  localai:
    image: localai/localai:v3.7.0
    ports: ["8080:8080"]
    environment:
      MODELS_PATH: /models
    volumes:
      - localai_models:/models
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/readyz || wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "litellm",
		Name:                   "LiteLLM Proxy",
		Slogan:                 "One OpenAI-format gateway in front of 100+ model providers, with keys, budgets, and spend tracking.",
		Category:               "AI",
		DocumentationURL:       "https://docs.litellm.ai/docs/simple_proxy",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  litellm:
    image: ghcr.io/berriai/litellm:main-stable
    ports: ["4000:4000"]
    environment:
      LITELLM_MASTER_KEY: sk-$SERVICE_HEX_32_MASTERKEY
      DATABASE_URL: postgres://litellm:$SERVICE_PASSWORD_DB@db:5432/litellm
      STORE_MODEL_IN_DB: "True"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:4000/health/liveliness || wget -q -O /dev/null http://127.0.0.1:4000/health/liveliness || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: litellm
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: litellm
    volumes:
      - litellm_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U litellm -d litellm"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "langfuse",
		Name:                   "Langfuse",
		Slogan:                 "Trace, evaluate, and debug LLM applications with prompt management and cost analytics.",
		Category:               "AI",
		DocumentationURL:       "https://langfuse.com/self-hosting",
		RecommendedMemoryBytes: 4294967296,
		// Tag of the bundled MinIO unverified: quay.io returns 401 anonymously.
		Compose: `services:
  langfuse-web:
    image: langfuse/langfuse:3.225.11
    ports: ["3000:3000"]
    environment:
      DATABASE_URL: postgres://langfuse:$SERVICE_PASSWORD_DB@db:5432/langfuse
      NEXTAUTH_URL: ${SERVICE_FQDN_LANGFUSE_WEB:-http://localhost:3000}
      NEXTAUTH_SECRET: $SERVICE_HEX_32_NEXTAUTH
      SALT: $SERVICE_HEX_32_SALT
      ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
      CLICKHOUSE_URL: http://clickhouse:8123
      CLICKHOUSE_MIGRATION_URL: clickhouse://clickhouse:9000
      CLICKHOUSE_USER: langfuse
      CLICKHOUSE_PASSWORD: $SERVICE_PASSWORD_CLICKHOUSE
      CLICKHOUSE_CLUSTER_ENABLED: "false"
      REDIS_HOST: redis
      LANGFUSE_S3_EVENT_UPLOAD_BUCKET: langfuse
      LANGFUSE_S3_EVENT_UPLOAD_REGION: auto
      LANGFUSE_S3_EVENT_UPLOAD_ENDPOINT: http://minio:9000
      LANGFUSE_S3_EVENT_UPLOAD_ACCESS_KEY_ID: langfuse
      LANGFUSE_S3_EVENT_UPLOAD_SECRET_ACCESS_KEY: $SERVICE_PASSWORD_MINIO
      LANGFUSE_S3_EVENT_UPLOAD_FORCE_PATH_STYLE: "true"
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api/public/health || wget -q -O /dev/null http://127.0.0.1:3000/api/public/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  langfuse-worker:
    image: langfuse/langfuse-worker:3.225.11
    environment:
      DATABASE_URL: postgres://langfuse:$SERVICE_PASSWORD_DB@db:5432/langfuse
      SALT: $SERVICE_HEX_32_SALT
      ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
      CLICKHOUSE_URL: http://clickhouse:8123
      CLICKHOUSE_MIGRATION_URL: clickhouse://clickhouse:9000
      CLICKHOUSE_USER: langfuse
      CLICKHOUSE_PASSWORD: $SERVICE_PASSWORD_CLICKHOUSE
      CLICKHOUSE_CLUSTER_ENABLED: "false"
      REDIS_HOST: redis
      LANGFUSE_S3_EVENT_UPLOAD_BUCKET: langfuse
      LANGFUSE_S3_EVENT_UPLOAD_REGION: auto
      LANGFUSE_S3_EVENT_UPLOAD_ENDPOINT: http://minio:9000
      LANGFUSE_S3_EVENT_UPLOAD_ACCESS_KEY_ID: langfuse
      LANGFUSE_S3_EVENT_UPLOAD_SECRET_ACCESS_KEY: $SERVICE_PASSWORD_MINIO
      LANGFUSE_S3_EVENT_UPLOAD_FORCE_PATH_STYLE: "true"
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: langfuse
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: langfuse
    volumes:
      - langfuse_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U langfuse -d langfuse"]
      interval: 10s
      timeout: 5s
      retries: 5
  clickhouse:
    image: clickhouse/clickhouse-server:24.12-alpine
    environment:
      CLICKHOUSE_USER: langfuse
      CLICKHOUSE_PASSWORD: $SERVICE_PASSWORD_CLICKHOUSE
    volumes:
      - langfuse_clickhouse_data:/var/lib/clickhouse
  redis:
    image: redis:7-alpine
    volumes:
      - langfuse_redis_data:/data
  minio:
    image: quay.io/minio/minio:RELEASE.2025-09-07T16-13-09Z
    command: ["sh", "-c", "mkdir -p /data/langfuse && minio server /data"]
    environment:
      MINIO_ROOT_USER: langfuse
      MINIO_ROOT_PASSWORD: $SERVICE_PASSWORD_MINIO
    volumes:
      - langfuse_minio_data:/data
`,
	},
	{
		ID:                     "flowise",
		Name:                   "Flowise",
		Slogan:                 "Drag-and-drop builder for LLM chatbots, agents, and RAG flows, with an API for each flow.",
		Category:               "AI",
		DocumentationURL:       "https://docs.flowiseai.com",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  flowise:
    image: flowiseai/flowise:3.1.4
    ports: ["3000:3000"]
    environment:
      PORT: "3000"
      FLOWISE_USERNAME: admin
      FLOWISE_PASSWORD: $SERVICE_PASSWORD_ADMIN
      DATABASE_PATH: /root/.flowise
      APIKEY_PATH: /root/.flowise
      SECRETKEY_PATH: /root/.flowise
      LOG_PATH: /root/.flowise/logs
      BLOB_STORAGE_PATH: /root/.flowise/storage
    volumes:
      - flowise_data:/root/.flowise
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3000/api/v1/ping || wget -q -O /dev/null http://127.0.0.1:3000/api/v1/ping || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "langflow",
		Name:                   "Langflow",
		Slogan:                 "A visual low-code tool for building and deploying multi-agent and RAG workflows in Python.",
		Category:               "AI",
		DocumentationURL:       "https://docs.langflow.org",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  langflow:
    image: langflowai/langflow:1.12.3
    ports: ["7860:7860"]
    environment:
      LANGFLOW_DATABASE_URL: postgresql://langflow:$SERVICE_PASSWORD_DB@db:5432/langflow
      LANGFLOW_CONFIG_DIR: /app/langflow
      LANGFLOW_AUTO_LOGIN: "false"
      LANGFLOW_SUPERUSER: admin
      LANGFLOW_SUPERUSER_PASSWORD: $SERVICE_PASSWORD_ADMIN
    volumes:
      - langflow_data:/app/langflow
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:7860/health_check || wget -q -O /dev/null http://127.0.0.1:7860/health_check || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 90s
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: langflow
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: langflow
    volumes:
      - langflow_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U langflow -d langflow"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "weaviate",
		Name:                   "Weaviate",
		Slogan:                 "An open-source vector database with hybrid search, built-in modules, and a GraphQL and REST API.",
		Category:               "Database Tools",
		DocumentationURL:       "https://weaviate.io/developers/weaviate",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  weaviate:
    image: semitechnologies/weaviate:1.39.6
    command: ["--host", "0.0.0.0", "--port", "8080", "--scheme", "http"]
    ports: ["8080:8080"]
    environment:
      PERSISTENCE_DATA_PATH: /var/lib/weaviate
      QUERY_DEFAULTS_LIMIT: "25"
      DEFAULT_VECTORIZER_MODULE: none
      CLUSTER_HOSTNAME: node1
      AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED: "false"
      AUTHENTICATION_APIKEY_ENABLED: "true"
      AUTHENTICATION_APIKEY_ALLOWED_KEYS: $SERVICE_HEX_32_APIKEY
      AUTHENTICATION_APIKEY_USERS: admin
    volumes:
      - weaviate_data:/var/lib/weaviate
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/v1/.well-known/ready || wget -q -O /dev/null http://127.0.0.1:8080/v1/.well-known/ready || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
}
