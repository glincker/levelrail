export interface ConnectStrings {
  uri: string
  cli: string
}

export interface ConnectInput {
  engine: string
  host: string
  port: number
  user: string
  database: string
  tls: boolean
}

const PASSWORD = '<password>'

/** Connection strings for one engine; the password is always a placeholder. */
export function connectStrings(i: ConnectInput): ConnectStrings {
  const addr = `${i.host}:${i.port}`
  switch (i.engine) {
    case 'postgres':
      return {
        uri: `postgresql://${i.user}:${PASSWORD}@${addr}/${i.database}?sslmode=${i.tls ? 'require' : 'prefer'}`,
        cli: `psql -h ${i.host} -p ${i.port} -U ${i.user} -d ${i.database}`,
      }
    case 'mysql':
    case 'mariadb':
      return {
        uri: `mysql://${i.user}:${PASSWORD}@${addr}/${i.database}${i.tls ? '?ssl-mode=REQUIRED' : ''}`,
        cli: `mysql -h ${i.host} -P ${i.port} -u ${i.user} -p${i.tls ? ' --ssl-mode=REQUIRED' : ''}`,
      }
    case 'redis':
      return {
        uri: `${i.tls ? 'rediss' : 'redis'}://:${PASSWORD}@${addr}`,
        cli: `redis-cli -h ${i.host} -p ${i.port}${i.tls ? ' --tls' : ''} -a`,
      }
    default:
      return { uri: addr, cli: '' }
  }
}

export function defaultConnectUser(engine: string): string {
  if (engine === 'postgres') return 'postgres'
  if (engine === 'mysql' || engine === 'mariadb') return 'root'
  return ''
}
