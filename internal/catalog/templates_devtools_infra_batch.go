package catalog

// catalogDevtoolsInfraTemplates holds backing services with no browser UI of
// their own (caches, brokers, KV stores), so they cannot be manifests, which
// require a domain prompt. All pins are explicit tags.
var catalogDevtoolsInfraTemplates = []Template{
	{
		ID:                     "valkey",
		Name:                   "Valkey",
		Slogan:                 "A Redis-compatible in-memory key-value store under the Linux Foundation, with password auth and append-only persistence.",
		Category:               "Database Tools",
		DocumentationURL:       "https://valkey.io/docs/",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  valkey:
    image: valkey/valkey:8.0-alpine
    ports: ["6379"]
    environment:
      VALKEY_PASSWORD: $SERVICE_PASSWORD_VALKEY
    command: ["/bin/sh", "-c", "exec valkey-server --requirepass \"$SERVICE_PASSWORD_VALKEY\" --appendonly yes"]
    volumes:
      - valkey_data:/data
    healthcheck:
      test: ["CMD-SHELL", "valkey-cli -a \"$SERVICE_PASSWORD_VALKEY\" ping | grep -q PONG || exit 1"]
      interval: 5s
      timeout: 5s
      retries: 10
`,
	},
	{
		ID:                     "memcached",
		Name:                   "Memcached",
		Slogan:                 "A fast in-memory cache for query results, sessions and rendered pages, reachable by other apps on host memcached.",
		Category:               "Database Tools",
		DocumentationURL:       "https://memcached.org/",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  memcached:
    image: memcached:1.6-alpine
    ports: ["11211"]
    command: ["memcached", "-m", "64"]
    healthcheck:
      test: ["CMD-SHELL", "echo version | nc -w 1 127.0.0.1 11211 | grep -q VERSION || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5
`,
	},
	{
		ID:                     "nats",
		Name:                   "NATS",
		Slogan:                 "A lightweight messaging server for pub/sub, request/reply and durable streams, with JetStream persistence enabled.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://docs.nats.io/",
		RecommendedMemoryBytes: 134217728,
		Compose: `services:
  nats:
    image: nats:2.10-alpine
    ports: ["4222", "8222"]
    command: ["-js", "-sd", "/data", "-m", "8222"]
    volumes:
      - nats_data:/data
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:8222/healthz"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 5s
`,
	},
	{
		ID:                     "etcd",
		Name:                   "etcd",
		Slogan:                 "A distributed key-value store for configuration and coordination, running as a single member.",
		Category:               "Infrastructure",
		DocumentationURL:       "https://etcd.io/docs/",
		RecommendedMemoryBytes: 268435456,
		Compose: `services:
  etcd:
    image: quay.io/coreos/etcd:v3.5.17
    ports: ["2379"]
    command: ["/usr/local/bin/etcd", "--name", "etcd0", "--data-dir", "/etcd-data", "--listen-client-urls", "http://0.0.0.0:2379", "--advertise-client-urls", "http://etcd:2379"]
    volumes:
      - etcd_data:/etcd-data
    healthcheck:
      test: ["CMD", "etcdctl", "endpoint", "health"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 5s
`,
	},
	{ //nolint:gosec // the password values below are compose magic-var tokens, not real credentials
		ID:                     "mongo-express",
		Name:                   "Mongo Express",
		Slogan:                 "A web admin console for MongoDB, bundled with its own MongoDB 7 instance and basic auth.",
		Category:               "Database Tools",
		DocumentationURL:       "https://github.com/mongo-express/mongo-express",
		RecommendedMemoryBytes: 536870912,
		Compose: `services:
  mongo-express:
    image: mongo-express:1.0.2-20-alpine3.19
    ports: ["8081"]
    environment:
      ME_CONFIG_MONGODB_URL: mongodb://root:$SERVICE_PASSWORD_MONGO@mongo:27017/
      ME_CONFIG_MONGODB_ENABLE_ADMIN: "true"
      ME_CONFIG_BASICAUTH_USERNAME: $SERVICE_USER_MONGOEXPRESS
      ME_CONFIG_BASICAUTH_PASSWORD: $SERVICE_PASSWORD_MONGOEXPRESS
    depends_on:
      mongo:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "nc -z 127.0.0.1 8081"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 10s
  mongo:
    image: mongo:7
    environment:
      MONGO_INITDB_ROOT_USERNAME: root
      MONGO_INITDB_ROOT_PASSWORD: $SERVICE_PASSWORD_MONGO
    volumes:
      - mongo_data:/data/db
    healthcheck:
      test: ["CMD-SHELL", "mongosh --quiet --eval \"db.adminCommand('ping').ok\" | grep -q 1"]
      interval: 10s
      timeout: 10s
      retries: 10
      start_period: 20s
`,
	},
}
