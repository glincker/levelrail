package catalog

var aiml2Templates = []Template{
	{
		ID:                     "chroma",
		Name:                   "Chroma",
		Slogan:                 "A developer-friendly embedding database for AI applications, served over a simple HTTP API.",
		Category:               "Database Tools",
		DocumentationURL:       "https://docs.trychroma.com",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  chroma:
    image: ghcr.io/chroma-core/chroma:1.0.0
    ports: ["8000:8000"]
    environment:
      IS_PERSISTENT: "TRUE"
      PERSIST_DIRECTORY: /data
      ANONYMIZED_TELEMETRY: "FALSE"
    volumes:
      - chroma_data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/api/v2/heartbeat || wget -q -O /dev/null http://127.0.0.1:8000/api/v2/heartbeat || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "milvus",
		Name:                   "Milvus",
		Slogan:                 "A cloud-native vector database built for billion-scale similarity search, run here as a single standalone node.",
		Category:               "Database Tools",
		DocumentationURL:       "https://milvus.io/docs",
		RecommendedMemoryBytes: 4294967296,
		Compose: `services:
  milvus:
    image: milvusdb/milvus:v3.0.2
    command: ["milvus", "run", "standalone"]
    ports: ["19530:19530", "9091:9091"]
    environment:
      ETCD_USE_EMBED: "true"
      ETCD_DATA_DIR: /var/lib/milvus/etcd
      COMMON_STORAGETYPE: local
    volumes:
      - milvus_data:/var/lib/milvus
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:9091/healthz || wget -q -O /dev/null http://127.0.0.1:9091/healthz || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 90s
`,
	},
	{
		ID:                     "speaches",
		Name:                   "Speaches",
		Slogan:                 "An OpenAI-compatible speech server: Whisper transcription and text-to-speech behind one API.",
		Category:               "AI",
		DocumentationURL:       "https://speaches.ai",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  speaches:
    image: ghcr.io/speaches-ai/speaches:0.8.3-cpu
    ports: ["8000:8000"]
    environment:
      ENABLE_UI: "true"
    volumes:
      - speaches_models:/home/ubuntu/.cache/huggingface/hub
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8000/health || wget -q -O /dev/null http://127.0.0.1:8000/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "jupyter-gpu",
		Name:                   "Jupyter GPU Notebook",
		Slogan:                 "JupyterLab with a CUDA-ready data science stack for training and experimenting with models.",
		Category:               "AI",
		DocumentationURL:       "https://github.com/iot-salzburg/gpu-jupyter",
		RecommendedMemoryBytes: 4294967296,
		RequiresGPU:            true,
		Compose: `services:
  jupyter:
    image: cschranz/gpu-jupyter:v1.11_cuda-13.0_ubuntu-24.04_python-only
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
    ports: ["8888:8888"]
    environment:
      JUPYTER_TOKEN: $SERVICE_PASSWORD_TOKEN
    volumes:
      - jupyter_work:/home/jovyan/work
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8888/api || wget -q -O /dev/null http://127.0.0.1:8888/api || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "mlflow",
		Name:                   "MLflow",
		Slogan:                 "Track experiments, compare runs, and manage models with a self-hosted MLflow tracking server.",
		Category:               "AI",
		DocumentationURL:       "https://mlflow.org/docs/latest",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  mlflow:
    image: ghcr.io/mlflow/mlflow:v3.16.1
    command: ["mlflow", "server", "--host", "0.0.0.0", "--port", "5000", "--backend-store-uri", "sqlite:////mlflow/mlflow.db", "--default-artifact-root", "/mlflow/artifacts"]
    ports: ["5000:5000"]
    volumes:
      - mlflow_data:/mlflow
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:5000/health')"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
`,
	},
	{
		ID:                     "label-studio",
		Name:                   "Label Studio",
		Slogan:                 "A flexible data labeling tool for text, images, audio, and video, with export to common ML formats.",
		Category:               "AI",
		DocumentationURL:       "https://labelstud.io/guide",
		RecommendedMemoryBytes: 1073741824,
		Compose: `services:
  label-studio:
    image: heartexlabs/label-studio:1.21.0
    ports: ["8080:8080"]
    environment:
      LABEL_STUDIO_USERNAME: admin@example.com
      LABEL_STUDIO_PASSWORD: $SERVICE_PASSWORD_ADMIN
      LABEL_STUDIO_DISABLE_SIGNUP_WITHOUT_LINK: "true"
    volumes:
      - label_studio_data:/label-studio/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:8080/health || wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "anythingllm",
		Name:                   "AnythingLLM",
		Slogan:                 "A private chat workspace over your documents, with built-in RAG and support for local or hosted models.",
		Category:               "AI",
		DocumentationURL:       "https://docs.anythingllm.com",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  anythingllm:
    image: mintplexlabs/anythingllm:1.16.2
    ports: ["3001:3001"]
    environment:
      SERVER_PORT: "3001"
      STORAGE_DIR: /app/server/storage
      JWT_SECRET: $SERVICE_HEX_32_JWT
    volumes:
      - anythingllm_storage:/app/server/storage
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3001/api/ping || wget -q -O /dev/null http://127.0.0.1:3001/api/ping || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
`,
	},
	{
		ID:                     "librechat",
		Name:                   "LibreChat",
		Slogan:                 "A self-hosted ChatGPT-style interface for many model providers, with agents, search, and multi-user login.",
		Category:               "AI",
		DocumentationURL:       "https://www.librechat.ai/docs",
		RecommendedMemoryBytes: 2147483648,
		Compose: `services:
  librechat:
    image: ghcr.io/danny-avila/librechat:v0.8.7
    ports: ["3080:3080"]
    environment:
      HOST: 0.0.0.0
      PORT: "3080"
      DOMAIN_CLIENT: ${SERVICE_FQDN_LIBRECHAT:-http://localhost:3080}
      DOMAIN_SERVER: ${SERVICE_FQDN_LIBRECHAT:-http://localhost:3080}
      MONGO_URI: mongodb://mongo:27017/LibreChat
      MEILI_HOST: http://meilisearch:7700
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MEILI
      JWT_SECRET: $SERVICE_HEX_32_JWT
      JWT_REFRESH_SECRET: $SERVICE_HEX_32_JWTREFRESH
      CREDS_KEY: $SERVICE_HEX_64_CREDSKEY
      CREDS_IV: $SERVICE_HEX_32_CREDSIV
      ALLOW_REGISTRATION: "true"
    volumes:
      - librechat_images:/app/client/public/images
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS -o /dev/null http://127.0.0.1:3080/health || wget -q -O /dev/null http://127.0.0.1:3080/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 60s
  mongo:
    image: mongo:7
    volumes:
      - librechat_mongo_data:/data/db
  meilisearch:
    image: getmeili/meilisearch:v1.54.0
    environment:
      MEILI_MASTER_KEY: $SERVICE_HEX_32_MEILI
      MEILI_NO_ANALYTICS: "true"
    volumes:
      - librechat_meili_data:/meili_data
`,
	},
	{
		ID:                     "n8n-ai-starter",
		Name:                   "n8n AI Starter Kit",
		Slogan:                 "n8n wired to a local Ollama model and a Qdrant vector store, ready for private AI workflows.",
		Category:               "AI",
		DocumentationURL:       "https://docs.n8n.io/advanced-ai",
		RecommendedMemoryBytes: 4294967296,
		Compose: `services:
  n8n:
    image: n8nio/n8n:1.62.1
    ports: ["5678:5678"]
    environment:
      N8N_ENCRYPTION_KEY: $SERVICE_HEX_64_ENCRYPTIONKEY
      DB_TYPE: postgresdb
      DB_POSTGRESDB_HOST: db
      DB_POSTGRESDB_DATABASE: n8n
      DB_POSTGRESDB_USER: n8n
      DB_POSTGRESDB_PASSWORD: $SERVICE_PASSWORD_DB
      OLLAMA_HOST: ollama:11434
    volumes:
      - n8n_ai_data:/home/node/.n8n
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:5678/healthz || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: n8n
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: n8n
    volumes:
      - n8n_ai_db_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U n8n -d n8n"]
      interval: 10s
      timeout: 5s
      retries: 5
  ollama:
    image: ollama/ollama:0.34.4
    volumes:
      - n8n_ai_ollama:/root/.ollama
  qdrant:
    image: qdrant/qdrant:v1.12.4
    volumes:
      - n8n_ai_qdrant:/qdrant/storage
`,
	},
	{
		ID:                     "pgvector",
		Name:                   "pgvector Postgres",
		Slogan:                 "PostgreSQL 17 with the pgvector extension preinstalled, for embeddings next to your relational data.",
		Category:               "Database Tools",
		DocumentationURL:       "https://github.com/pgvector/pgvector",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  pgvector:
    image: pgvector/pgvector:0.8.6-pg17-trixie
    ports: ["5432:5432"]
    environment:
      POSTGRES_USER: vector
      POSTGRES_PASSWORD: $SERVICE_PASSWORD_DB
      POSTGRES_DB: vector
    volumes:
      - pgvector_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U vector -d vector"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
}
