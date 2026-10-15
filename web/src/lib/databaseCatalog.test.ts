import { describe, expect, it } from 'vitest'
import { buildPickerEntries, engineHasBackups } from './databaseCatalog'

const ENGINES = [
  { id: 'postgres', label: 'Postgres' },
  { id: 'redis', label: 'Redis' },
  { id: 'meilisearch', label: 'Meilisearch' },
  { id: 'qdrant', label: 'Qdrant' },
  { id: 'typesense', label: 'Typesense' },
  { id: 'memcached', label: 'Memcached' },
]
const BRANDS = [
  'postgres',
  'redis',
  'meilisearch',
  'qdrant',
  'typesense',
  'memcached',
]

describe('buildPickerEntries for search engines and Memcached', () => {
  const byKey = new Map(
    buildPickerEntries(ENGINES, BRANDS).map((e) => [e.key, e]),
  )

  it.each([
    ['meilisearch', 'search', 7700, 'snapshot', true],
    ['qdrant', 'search', 6333, 'snapshot', true],
    ['typesense', 'search', 8108, 'snapshot', true],
    ['pgvector', 'search', 5432, 'dump', false],
    ['memcached', 'keyvalue', 11211, 'none', false],
    ['redis', 'keyvalue', 6379, 'dump', false],
  ] as const)('%s', (key, family, port, backup, httpApi) => {
    const entry = byKey.get(key)
    expect(entry).toMatchObject({ family, port, backup, httpApi })
  })

  it('uses the brand mark when the slug exists', () => {
    expect(byKey.get('qdrant')?.brand).toBe('qdrant')
    expect(byKey.get('memcached')?.brand).toBe('memcached')
  })

  it('reports which engines can be backed up', () => {
    expect(engineHasBackups('memcached')).toBe(false)
    expect(engineHasBackups('meilisearch')).toBe(true)
    expect(engineHasBackups('postgres')).toBe(true)
  })
})
