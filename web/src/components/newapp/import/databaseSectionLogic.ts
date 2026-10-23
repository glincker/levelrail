import type { DatabaseEdit } from '../../../lib/newApp'
import type { DatabaseEngineInfo } from '../../../queries/databaseEngines'
import {
  backupRetain,
  scheduleCron,
  suggestName,
} from '../../databaseFlow/logic'

/** Engines offered as one-click chips, in display order. */
export const CHIP_ENGINES = ['postgres', 'mysql', 'redis', 'mongodb'] as const

const ENV_VAR_BY_ENGINE: Record<string, string> = {
  postgres: 'DATABASE_URL',
  mysql: 'DATABASE_URL',
  mariadb: 'DATABASE_URL',
  redis: 'REDIS_URL',
  valkey: 'REDIS_URL',
  keydb: 'REDIS_URL',
  dragonfly: 'REDIS_URL',
  mongodb: 'MONGODB_URI',
}

const NAME_MAX = 63

export function envVarFor(engine: string): string {
  return ENV_VAR_BY_ENGINE[engine] ?? 'DATABASE_URL'
}

/** Chips for the engines the registry lists (all of them while it loads). */
export function chipEngines(
  registry: readonly DatabaseEngineInfo[],
  fallback: readonly DatabaseEngineInfo[],
): DatabaseEngineInfo[] {
  const source = registry.length > 0 ? registry : fallback
  return CHIP_ENGINES.flatMap((id) => source.filter((e) => e.id === id))
}

/** "<app>-<engine>", numbered when taken, always a valid resource name. */
export function suggestDbName(
  appName: string,
  engine: string,
  taken: readonly string[],
): string {
  const app = appName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, '-')
    .replace(/^-+|-+$/g, '')
  if (app === '') return suggestName(engine, taken)
  const room = NAME_MAX - engine.length - 4
  return suggestName(`${app.slice(0, room)}-${engine}`, taken)
}

export interface BackupTargetRef {
  id: string
}

export function newDatabaseEdit(input: {
  engine: string
  version: string
  name: string
  backups: boolean
  target: BackupTargetRef | undefined
  envVar?: string
  field?: string
}): DatabaseEdit {
  return {
    enabled: true,
    engine: input.engine,
    version: input.version.trim(),
    name: input.name.trim(),
    envVar: input.envVar ?? envVarFor(input.engine),
    field: input.field ?? 'url',
    ...(input.backups && input.target
      ? {
          backup: {
            targetId: input.target.id,
            schedule: scheduleCron('daily'),
            retain: backupRetain(),
          },
        }
      : {}),
  }
}

export function existingDatabaseEdit(db: {
  name: string
  engine: string
}): DatabaseEdit {
  return {
    enabled: true,
    engine: db.engine,
    version: '',
    name: db.name,
    envVar: envVarFor(db.engine),
    field: 'url',
  }
}

/** The variables Deploy will inject into the new app, for the masked list. */
export function injectedVars(
  dbs: readonly DatabaseEdit[],
): { key: string; database: string }[] {
  const seen = new Set<string>()
  const out: { key: string; database: string }[] = []
  for (const d of dbs) {
    if (!d.enabled || seen.has(d.envVar)) continue
    seen.add(d.envVar)
    out.push({ key: d.envVar, database: d.name })
  }
  return out
}
