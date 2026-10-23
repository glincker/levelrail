export interface ProvisionConnection {
  host: string
  port: number
  scheme: string
}

const ENGINE_PORTS: Record<string, { port: number; scheme: string }> = {
  postgres: { port: 5432, scheme: 'postgres' },
  mysql: { port: 3306, scheme: 'mysql' },
  mariadb: { port: 3306, scheme: 'mysql' },
  mongodb: { port: 27017, scheme: 'mongodb' },
  redis: { port: 6379, scheme: 'redis' },
  valkey: { port: 6379, scheme: 'redis' },
  keydb: { port: 6379, scheme: 'redis' },
  dragonfly: { port: 6379, scheme: 'redis' },
}

export function connectionFor(
  name: string,
  engine: string | undefined,
): ProvisionConnection | undefined {
  const e = engine ? ENGINE_PORTS[engine] : undefined
  if (!e) return undefined
  return { host: `db-${name}`, port: e.port, scheme: e.scheme }
}
