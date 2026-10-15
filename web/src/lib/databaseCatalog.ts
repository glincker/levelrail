import type { BrandIconName } from '@/components/BrandIcon'

export type DatabaseFamily =
  'relational' | 'keyvalue' | 'document' | 'analytics' | 'search' | 'other'

export const DATABASE_FAMILIES: readonly DatabaseFamily[] = [
  'relational',
  'keyvalue',
  'document',
  'analytics',
  'search',
  'other',
]

/** How an engine's data is backed up: a dump file, an engine snapshot, or
 *  nothing at all for a cache that keeps no data on disk. */
export type BackupKind = 'dump' | 'snapshot' | 'none'

export type PostgresFlavor = 'pgvector' | 'timescaledb' | 'postgis'

export const POSTGRES_FLAVORS: readonly PostgresFlavor[] = [
  'pgvector',
  'timescaledb',
  'postgis',
]

// Mirrors internal/reconcile/database/variant.go: the version string is
// "<major>-<flavor>" and the engine id stays "postgres".
const FLAVOR_MIN_MAJOR: Record<PostgresFlavor, number> = {
  pgvector: 13,
  timescaledb: 15,
  postgis: 13,
}

const DEFAULT_POSTGRES_MAJOR = 17

export function flavorOfVersion(version: string): PostgresFlavor | null {
  const lower = version.trim().toLowerCase()
  return POSTGRES_FLAVORS.find((f) => lower.endsWith(`-${f}`)) ?? null
}

// Keeps the leading major digits and drops any minor part or old flavor,
// since the API accepts only "<major>-<flavor>".
export function withFlavor(
  version: string,
  flavor: PostgresFlavor | null,
): string {
  const trimmed = version.trim()
  const current = flavorOfVersion(trimmed)
  if (flavor === null) {
    return current ? trimmed.slice(0, -(current.length + 1)) : trimmed
  }
  const parsed = /^\d+/.exec(trimmed)?.[0]
  const major = Math.max(
    parsed ? Number(parsed) : DEFAULT_POSTGRES_MAJOR,
    FLAVOR_MIN_MAJOR[flavor],
  )
  return `${major}-${flavor}`
}

export interface PickerEntry {
  /** Unique within the picker: the engine id, or the flavor name. */
  key: string
  engineId: string
  flavor: PostgresFlavor | null
  family: DatabaseFamily
  brand: BrandIconName | null
  port: number | null
  compat: 'redis' | 'mysql' | 'postgres' | null
  /** The label shown on the card: the registry label, or the flavor's. */
  label: string
  backup: BackupKind
  httpApi: boolean
}

interface EngineTraits {
  family: DatabaseFamily
  port: number
  compat: PickerEntry['compat']
  backup?: BackupKind
  httpApi?: boolean
}

const ENGINE_TRAITS: Record<string, EngineTraits> = {
  postgres: { family: 'relational', port: 5432, compat: null },
  mysql: { family: 'relational', port: 3306, compat: null },
  mariadb: { family: 'relational', port: 3306, compat: 'mysql' },
  redis: { family: 'keyvalue', port: 6379, compat: null },
  valkey: { family: 'keyvalue', port: 6379, compat: 'redis' },
  keydb: { family: 'keyvalue', port: 6379, compat: 'redis' },
  dragonfly: { family: 'keyvalue', port: 6379, compat: 'redis' },
  mongodb: { family: 'document', port: 27017, compat: null },
  clickhouse: { family: 'analytics', port: 8123, compat: null },
  memcached: { family: 'keyvalue', port: 11211, compat: null, backup: 'none' },
  meilisearch: {
    family: 'search',
    port: 7700,
    compat: null,
    backup: 'snapshot',
    httpApi: true,
  },
  qdrant: {
    family: 'search',
    port: 6333,
    compat: null,
    backup: 'snapshot',
    httpApi: true,
  },
  typesense: {
    family: 'search',
    port: 8108,
    compat: null,
    backup: 'snapshot',
    httpApi: true,
  },
}

/** Whether engine keeps data a backup can capture (Memcached does not). */
export function engineHasBackups(engineId: string): boolean {
  return ENGINE_TRAITS[engineId]?.backup !== 'none'
}

interface FlavorTraits {
  family: DatabaseFamily
  brand: BrandIconName | null
  label: string
}

const FLAVOR_TRAITS: Record<PostgresFlavor, FlavorTraits> = {
  pgvector: { family: 'search', brand: null, label: 'pgvector' },
  timescaledb: {
    family: 'analytics',
    brand: 'timescaledb',
    label: 'TimescaleDB',
  },
  postgis: { family: 'relational', brand: null, label: 'PostGIS' },
}

function brandFor(
  engineId: string,
  names: readonly string[],
): BrandIconName | null {
  return names.includes(engineId) ? (engineId as BrandIconName) : null
}

/** Picker cards: one per registry engine, plus a card per Postgres flavor
 *  whenever the registry offers Postgres. */
export function buildPickerEntries(
  engines: readonly { id: string; label: string }[],
  brandNames: readonly string[],
): PickerEntry[] {
  const entries: PickerEntry[] = []
  for (const engine of engines) {
    const traits = ENGINE_TRAITS[engine.id]
    entries.push({
      key: engine.id,
      engineId: engine.id,
      flavor: null,
      family: traits?.family ?? 'other',
      brand: brandFor(engine.id, brandNames),
      port: traits?.port ?? null,
      compat: traits?.compat ?? null,
      label: engine.label,
      backup: traits?.backup ?? 'dump',
      httpApi: traits?.httpApi ?? false,
    })
    if (engine.id === 'postgres') {
      for (const flavor of POSTGRES_FLAVORS) {
        const f = FLAVOR_TRAITS[flavor]
        entries.push({
          key: flavor,
          engineId: 'postgres',
          flavor,
          family: f.family,
          brand: f.brand,
          port: 5432,
          compat: 'postgres',
          label: f.label,
          backup: 'dump',
          httpApi: false,
        })
      }
    }
  }
  return entries
}

export function flavorLabel(flavor: PostgresFlavor): string {
  return FLAVOR_TRAITS[flavor].label
}
