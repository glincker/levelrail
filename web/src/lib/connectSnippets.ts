// Pure generators behind the database Connect dialog. The password is a
// placeholder unless the caller passes a revealed one.

export const CONNECT_PASSWORD_PLACEHOLDER = '<password>'

export type ConnectFamily = 'postgres' | 'mysql' | 'mongodb' | 'redis' | 'other'

export type ConnectFramework =
  | 'url'
  | 'env'
  | 'springProperties'
  | 'springYaml'
  | 'node'
  | 'python'
  | 'go'
  | 'dotnet'

export interface ConnectInput {
  engine: string
  host: string
  port: number
  database: string
  user: string
  // null keeps the placeholder in every output.
  password: string | null
  tls: boolean
}

export type ConnectSnippets = Partial<Record<ConnectFramework, string>>

const REDIS_ENGINES = new Set(['redis', 'valkey', 'keydb', 'dragonfly'])

export function connectFamily(engine: string): ConnectFamily {
  if (engine === 'postgres') return 'postgres'
  if (engine === 'mysql' || engine === 'mariadb') return 'mysql'
  if (engine === 'mongodb') return 'mongodb'
  if (REDIS_ENGINES.has(engine)) return 'redis'
  return 'other'
}

/** Frameworks that make sense for an engine, in tab order. */
export function connectFrameworks(engine: string): ConnectFramework[] {
  if (connectFamily(engine) === 'other') return ['url', 'env']
  return [
    'springProperties',
    'springYaml',
    'node',
    'python',
    'go',
    'dotnet',
    'url',
    'env',
  ]
}

function pw(i: ConnectInput): string {
  return i.password ?? CONNECT_PASSWORD_PLACEHOLDER
}

// A revealed password is percent-encoded inside URLs; the placeholder stays readable.
function pwUrl(i: ConnectInput): string {
  return i.password === null
    ? CONNECT_PASSWORD_PLACEHOLDER
    : encodeURIComponent(i.password)
}

function userUrl(i: ConnectInput): string {
  return encodeURIComponent(i.user)
}

export function envVarName(engine: string): string {
  const f = connectFamily(engine)
  if (f === 'mongodb') return 'MONGODB_URI'
  if (f === 'redis') return 'REDIS_URL'
  return 'DATABASE_URL'
}

/** Driver agnostic URL, the one to paste into a client. */
export function connectUrl(i: ConnectInput): string {
  const addr = `${i.host}:${i.port}`
  switch (connectFamily(i.engine)) {
    case 'postgres':
      return `postgresql://${userUrl(i)}:${pwUrl(i)}@${addr}/${i.database}?sslmode=${i.tls ? 'require' : 'prefer'}`
    case 'mysql':
      return `mysql://${userUrl(i)}:${pwUrl(i)}@${addr}/${i.database}${i.tls ? '?ssl-mode=REQUIRED' : ''}`
    case 'mongodb':
      return `mongodb://${userUrl(i)}:${pwUrl(i)}@${addr}/${i.database}?authSource=admin&directConnection=true${i.tls ? '&tls=true' : ''}`
    case 'redis': {
      const auth = i.user ? `${userUrl(i)}:${pwUrl(i)}` : `:${pwUrl(i)}`
      return `${i.tls ? 'rediss' : 'redis'}://${auth}@${addr}`
    }
    default:
      return addr
  }
}

function jdbcUrl(i: ConnectInput): string | null {
  const addr = `${i.host}:${i.port}/${i.database}`
  if (i.engine === 'postgres') {
    return `jdbc:postgresql://${addr}${i.tls ? '?sslmode=require' : ''}`
  }
  if (i.engine === 'mariadb') {
    return `jdbc:mariadb://${addr}${i.tls ? '?sslMode=trust' : ''}`
  }
  if (i.engine === 'mysql') {
    return `jdbc:mysql://${addr}${i.tls ? '?sslMode=REQUIRED' : ''}`
  }
  return null
}

function springProperties(i: ConnectInput): string {
  const jdbc = jdbcUrl(i)
  if (jdbc) {
    return [
      `spring.datasource.url=${jdbc}`,
      `spring.datasource.username=${i.user}`,
      `spring.datasource.password=${pw(i)}`,
    ].join('\n')
  }
  if (connectFamily(i.engine) === 'mongodb') {
    return `spring.data.mongodb.uri=${connectUrl(i)}`
  }
  const lines = [
    `spring.data.redis.host=${i.host}`,
    `spring.data.redis.port=${i.port}`,
  ]
  if (i.user) lines.push(`spring.data.redis.username=${i.user}`)
  lines.push(`spring.data.redis.password=${pw(i)}`)
  if (i.tls) lines.push('spring.data.redis.ssl.enabled=true')
  return lines.join('\n')
}

function q(value: string): string {
  return `"${value.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"`
}

function springYaml(i: ConnectInput): string {
  const jdbc = jdbcUrl(i)
  if (jdbc) {
    return [
      'spring:',
      '  datasource:',
      `    url: ${q(jdbc)}`,
      `    username: ${q(i.user)}`,
      `    password: ${q(pw(i))}`,
    ].join('\n')
  }
  if (connectFamily(i.engine) === 'mongodb') {
    return [
      'spring:',
      '  data:',
      '    mongodb:',
      `      uri: ${q(connectUrl(i))}`,
    ].join('\n')
  }
  const lines = [
    'spring:',
    '  data:',
    '    redis:',
    `      host: ${i.host}`,
    `      port: ${i.port}`,
  ]
  if (i.user) lines.push(`      username: ${q(i.user)}`)
  lines.push(`      password: ${q(pw(i))}`)
  if (i.tls) lines.push('      ssl:', '        enabled: true')
  return lines.join('\n')
}

function nodeSnippet(i: ConnectInput): string {
  const env = envVarName(i.engine)
  switch (connectFamily(i.engine)) {
    case 'postgres':
      return `import pg from 'pg'\n\nconst client = new pg.Client({ connectionString: process.env.${env} })\nawait client.connect()`
    case 'mysql':
      return `import mysql from 'mysql2/promise'\n\nconst conn = await mysql.createConnection(process.env.${env})`
    case 'mongodb':
      return `import mongoose from 'mongoose'\n\nawait mongoose.connect(process.env.${env})`
    default:
      return `import Redis from 'ioredis'\n\nconst redis = new Redis(process.env.${env})`
  }
}

function pythonSnippet(i: ConnectInput): string {
  const env = envVarName(i.engine)
  switch (connectFamily(i.engine)) {
    case 'postgres':
      return `import os\nimport psycopg\n\nconn = psycopg.connect(os.environ["${env}"])`
    case 'mysql':
      return `import os\nfrom urllib.parse import urlparse\nimport pymysql\n\nu = urlparse(os.environ["${env}"])\nconn = pymysql.connect(host=u.hostname, port=u.port, user=u.username, password=u.password, database=u.path.lstrip("/"))`
    case 'mongodb':
      return `import os\nfrom pymongo import MongoClient\n\nclient = MongoClient(os.environ["${env}"])`
    default:
      return `import os\nimport redis\n\nr = redis.from_url(os.environ["${env}"])`
  }
}

function goSnippet(i: ConnectInput): string {
  const env = envVarName(i.engine)
  switch (connectFamily(i.engine)) {
    case 'postgres':
      return `conn, err := pgx.Connect(ctx, os.Getenv("${env}"))\nif err != nil {\n\tlog.Fatal(err)\n}`
    case 'mysql':
      return `// go-sql-driver DSN, not a URL\ndsn := fmt.Sprintf("%s:%s@tcp(${i.host}:${i.port})/${i.database}", user, password)\ndb, err := sql.Open("mysql", dsn)\nif err != nil {\n\tlog.Fatal(err)\n}`
    case 'mongodb':
      return `client, err := mongo.Connect(options.Client().ApplyURI(os.Getenv("${env}")))\nif err != nil {\n\tlog.Fatal(err)\n}`
    default:
      return `opt, err := redis.ParseURL(os.Getenv("${env}"))\nif err != nil {\n\tlog.Fatal(err)\n}\nclient := redis.NewClient(opt)`
  }
}

function dotnetSnippet(i: ConnectInput): string {
  let conn: string
  switch (connectFamily(i.engine)) {
    case 'postgres':
      conn = `Host=${i.host};Port=${i.port};Database=${i.database};Username=${i.user};Password=${pw(i)};SSL Mode=${i.tls ? 'Require' : 'Prefer'}`
      break
    case 'mysql':
      conn = `Server=${i.host};Port=${i.port};Database=${i.database};User=${i.user};Password=${pw(i)};SslMode=${i.tls ? 'Required' : 'Preferred'}`
      break
    case 'mongodb':
      conn = connectUrl(i)
      break
    default:
      conn = `${i.host}:${i.port}${i.user ? `,user=${i.user}` : ''},password=${pw(i)}${i.tls ? ',ssl=true' : ''}`
  }
  return JSON.stringify({ ConnectionStrings: { Default: conn } }, null, 2)
}

/** The .env block: the URL plus the discrete parts for tools that want them. */
export function envFile(i: ConnectInput): string {
  const lines = [`${envVarName(i.engine)}=${connectUrl(i)}`]
  lines.push(`DB_HOST=${i.host}`, `DB_PORT=${i.port}`)
  if (i.database) lines.push(`DB_NAME=${i.database}`)
  if (i.user) lines.push(`DB_USER=${i.user}`)
  lines.push(`DB_PASSWORD=${pw(i)}`)
  return lines.join('\n')
}

export function buildConnectSnippets(i: ConnectInput): ConnectSnippets {
  const out: ConnectSnippets = { url: connectUrl(i), env: envFile(i) }
  if (connectFamily(i.engine) === 'other') return out
  out.springProperties = springProperties(i)
  out.springYaml = springYaml(i)
  out.node = nodeSnippet(i)
  out.python = pythonSnippet(i)
  out.go = goSnippet(i)
  out.dotnet = dotnetSnippet(i)
  return out
}

export type ConnectDownload = 'env' | 'properties' | 'yaml'

export const CONNECT_DOWNLOAD_FILES: Record<ConnectDownload, string> = {
  env: '.env',
  properties: 'application.properties',
  yaml: 'application.yml',
}

/** Content of a downloadable file, or null when the engine has no such config. */
export function downloadContent(
  kind: ConnectDownload,
  i: ConnectInput,
): string | null {
  const s = buildConnectSnippets(i)
  const text =
    kind === 'env'
      ? s.env
      : kind === 'properties'
        ? s.springProperties
        : s.springYaml
  return text ? `${text}\n` : null
}
