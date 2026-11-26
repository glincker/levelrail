export const SQL_ENGINES = new Set(['postgres', 'mysql', 'mariadb'])
export const KV_ENGINES = new Set(['redis', 'keydb', 'dragonfly', 'valkey'])
export const DOCUMENT_ENGINES = new Set(['mongodb'])
export const TABLE_ENGINES = new Set(['dynamodb'])
// No browser: the explorer shows live counts from the engine's own API.
export const STATS_ENGINES = new Set([
  'meilisearch',
  'qdrant',
  'typesense',
  'memcached',
])

// The server explorer only speaks to MongoDB, so DocumentDB has no browser yet.
export const EXPLORER_UNAVAILABLE_ENGINES = new Set(['documentdb'])

export function hasExplorer(engine: string): boolean {
  return [
    SQL_ENGINES,
    KV_ENGINES,
    DOCUMENT_ENGINES,
    TABLE_ENGINES,
    STATS_ENGINES,
  ].some((set) => set.has(engine))
}
