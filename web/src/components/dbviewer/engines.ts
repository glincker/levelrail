export const SQL_ENGINES = new Set(['postgres', 'mysql', 'mariadb'])
export const KV_ENGINES = new Set(['redis', 'keydb', 'dragonfly', 'valkey'])
// No browser: the explorer shows live counts from the engine's own API.
export const STATS_ENGINES = new Set([
  'meilisearch',
  'qdrant',
  'typesense',
  'memcached',
])
