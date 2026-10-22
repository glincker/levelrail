import type { ComposePreviewService } from '../types/compose'

export interface TemplateSummaryInput {
  compose: string
  services: ComposePreviewService[]
  recommendedMemoryBytes?: number
}

export interface TemplateSummary {
  ports: number[]
  memoryBytes?: number
  cpus?: number
  volumes: string[]
  databases: string[]
  hasBackups: boolean
  serviceCount: number
}

const DB_IMAGE =
  /(?:^|\/)(postgres|postgis|mysql|mariadb|mongo|redis|valkey|clickhouse|timescaledb)(?::|@|$)/

const UNITS: Record<string, number> = {
  b: 1,
  k: 1024,
  kb: 1024,
  m: 1024 ** 2,
  mb: 1024 ** 2,
  g: 1024 ** 3,
  gb: 1024 ** 3,
}

/** parseMemory reads a compose size such as "512m" or "1.5g" into bytes. */
export function parseMemory(value: string): number | undefined {
  const m = /^\s*(\d+(?:\.\d+)?)\s*([a-z]*)\s*$/i.exec(value)
  if (!m) return undefined
  const unit = UNITS[(m[2] ?? '').toLowerCase()]
  if (unit === undefined) return undefined
  return Math.round(Number(m[1]) * unit)
}

function firstMatch(compose: string, key: string): string | undefined {
  const m = new RegExp(`^\\s*${key}:\\s*["']?([^"'\\s#]+)`, 'm').exec(compose)
  return m?.[1]
}

/** databaseImageName returns the engine name for a database image, else undefined. */
export function databaseImageName(
  image: string | undefined,
): string | undefined {
  return image ? DB_IMAGE.exec(image)?.[1] : undefined
}

/** computeTemplateSummary derives the "what you will get" facts from the template definition alone. */
export function computeTemplateSummary(
  input: TemplateSummaryInput,
): TemplateSummary {
  const ports = Array.from(
    new Set(input.services.flatMap((s) => (s.port ? [s.port] : []))),
  ).sort((a, b) => a - b)
  const volumes = Array.from(
    new Set(input.services.flatMap((s) => s.volumes ?? [])),
  )
  const databases = Array.from(
    new Set(
      input.services.flatMap((s) => {
        const name = databaseImageName(s.image)
        return name ? [name] : []
      }),
    ),
  )
  const limit = firstMatch(input.compose, 'mem_limit')
  const memoryBytes =
    (limit ? parseMemory(limit) : undefined) ?? input.recommendedMemoryBytes
  const cpuRaw = firstMatch(input.compose, 'cpus')
  const cpus =
    cpuRaw && Number.isFinite(Number(cpuRaw)) ? Number(cpuRaw) : undefined
  return {
    ports,
    memoryBytes: memoryBytes || undefined,
    cpus,
    volumes,
    databases,
    hasBackups: volumes.length > 0,
    serviceCount: input.services.length,
  }
}

/** appSlug mirrors the app name rule the install endpoint accepts. */
export function appSlug(id: string, name: string): string {
  const base = (name.trim() || id).toLowerCase().replace(/[^a-z0-9-]+/g, '-')
  return base.replace(/^-+|-+$/g, '').slice(0, 40) || id
}

/** primaryVersion is the tag of the first service image, when it is a real version. */
export function primaryVersion(
  services: ComposePreviewService[],
): string | undefined {
  const tag = /:([^:/@]+)(?:@.*)?$/.exec(services[0]?.image ?? '')?.[1]
  return tag && tag !== 'latest' ? tag : undefined
}
