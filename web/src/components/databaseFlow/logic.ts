import type { ServiceResources } from '../../types/appDetail'
import type { DatabaseEngineInfo } from '../../queries/databaseEngines'

export type PresetId = 'small' | 'medium' | 'large'
export type ScheduleId = 'daily' | 'sixHourly' | 'weekly'
export type NameError = 'required' | 'format' | 'taken'

export interface DbValues {
  engine: string
  version: string
  name: string
  backups: boolean
  schedule: ScheduleId
  preset: PresetId
  nodeId: string
  projectId: string
}

const NAME_PATTERN = /^[a-z0-9][a-z0-9-]{0,62}$/
const MIB = 1024 * 1024
const DEFAULT_RETAIN = 7

interface Preset {
  id: PresetId
  memoryMiB: number
  cpus: number
}

const SMALL: Preset = { id: 'small', memoryMiB: 512, cpus: 0.5 }
export const PRESETS: readonly Preset[] = [
  SMALL,
  { id: 'medium', memoryMiB: 1024, cpus: 1 },
  { id: 'large', memoryMiB: 2048, cpus: 2 },
]

interface Schedule {
  id: ScheduleId
  cron: string
}

const DAILY: Schedule = { id: 'daily', cron: '0 3 * * *' }
export const SCHEDULES: readonly Schedule[] = [
  DAILY,
  { id: 'sixHourly', cron: '0 */6 * * *' },
  { id: 'weekly', cron: '0 3 * * 0' },
]

export const SCHEDULE_LABEL = {
  daily: 'flow.schedules.daily',
  sixHourly: 'flow.schedules.sixHourly',
  weekly: 'flow.schedules.weekly',
} as const satisfies Record<ScheduleId, string>

const HEAVY_ENGINES = new Set(['clickhouse', 'mongodb'])
const NO_BACKUP_ENGINES = new Set(['memcached'])
export const CREDENTIALED_ENGINES = new Set([
  'postgres',
  'mysql',
  'mongodb',
  'mariadb',
  'clickhouse',
])

export function defaultPreset(engine: string): PresetId {
  return HEAVY_ENGINES.has(engine) ? 'medium' : 'small'
}

export function supportsBackups(engine: string): boolean {
  return !NO_BACKUP_ENGINES.has(engine)
}

export function presetResources(id: PresetId): ServiceResources {
  const p = PRESETS.find((x) => x.id === id) ?? SMALL
  return {
    memory_bytes: p.memoryMiB * MIB,
    nano_cpus: Math.round(p.cpus * 1e9),
  }
}

export function scheduleCron(id: ScheduleId): string {
  return (SCHEDULES.find((s) => s.id === id) ?? DAILY).cron
}

export function backupRetain(): number {
  const raw = Number(
    import.meta.env.VITE_DATABASE_BACKUP_RETAIN ?? DEFAULT_RETAIN,
  )
  return Number.isInteger(raw) && raw > 0 ? raw : DEFAULT_RETAIN
}

export function suggestName(engine: string, taken: readonly string[]): string {
  if (engine === '') return ''
  const used = new Set(taken)
  if (!used.has(engine)) return engine
  for (let n = 2; ; n += 1) {
    const candidate = `${engine}-${n}`
    if (!used.has(candidate)) return candidate
  }
}

export function validateName(
  name: string,
  taken: readonly string[],
): NameError | undefined {
  const value = name.trim()
  if (value === '') return 'required'
  if (!NAME_PATTERN.test(value)) return 'format'
  if (taken.includes(value)) return 'taken'
  return undefined
}

export interface DbErrors {
  name?: NameError
  version?: 'required'
}

export function validate(v: DbValues, taken: readonly string[]): DbErrors {
  const errors: DbErrors = {}
  const name = validateName(v.name, taken)
  if (name) errors.name = name
  if (v.version.trim() === '') errors.version = 'required'
  return errors
}

export function pickVersion(
  current: string,
  touched: boolean,
  versions: readonly { version: string; recommended: boolean }[],
  fallback: string,
): string {
  if (touched && current.trim() !== '') return current
  return versions.find((x) => x.recommended)?.version ?? fallback
}

export function summaryKey(v: DbValues, backupsAvailable: boolean) {
  const withBackups = v.backups && supportsBackups(v.engine) && backupsAvailable
  return withBackups ? 'flow.summary.withBackups' : 'flow.summary.noBackups'
}

export const FALLBACK_ENGINES: readonly DatabaseEngineInfo[] = [
  { id: 'postgres', label: 'Postgres', default_version: '16' },
  { id: 'redis', label: 'Redis', default_version: '7' },
  { id: 'mysql', label: 'MySQL', default_version: '8' },
  { id: 'mongodb', label: 'MongoDB', default_version: '7' },
  { id: 'mariadb', label: 'MariaDB', default_version: '11' },
  { id: 'keydb', label: 'KeyDB', default_version: 'latest' },
  { id: 'dragonfly', label: 'Dragonfly', default_version: 'v1.27.1' },
  { id: 'valkey', label: 'Valkey', default_version: '8' },
]

export const DEFAULT_ENGINE: DatabaseEngineInfo = {
  id: 'postgres',
  label: 'Postgres',
  default_version: '16',
}
