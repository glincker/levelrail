const AURORA_PREFIX = 'aurora'

export const RDS_ENGINES = [
  'postgres',
  'mysql',
  'mariadb',
  'aurora-postgresql',
  'aurora-mysql',
] as const

// Multi-AZ and Aurora cost more and so need a typed confirm before create.
export function rdsNeedsTypedConfirm(
  engine: string,
  multiAz: boolean,
): boolean {
  return multiAz || engine.startsWith(AURORA_PREFIX)
}

export function rdsCanStop(i: {
  managed: boolean
  status: string
  engine: string
}): boolean {
  return (
    i.managed && i.status === 'available' && !i.engine.startsWith(AURORA_PREFIX)
  )
}

export function rdsCanStart(i: { managed: boolean; status: string }): boolean {
  return i.managed && i.status === 'stopped'
}
