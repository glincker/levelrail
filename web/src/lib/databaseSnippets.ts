import { connectStrings } from './databaseConnect'

export const MASK = '••••••••'
export const PASSWORD_PLACEHOLDER = '<password>'

export type SnippetTab = 'uri' | 'env' | 'node' | 'python' | 'go' | 'cli'

export interface SnippetInput {
  engine: string
  host: string
  port: number
  user: string
  database: string
  tls: boolean
  revealPlaceholder: boolean
}

/** Snippets never contain a real password: only a mask or the literal placeholder. */
export function buildSnippets(i: SnippetInput): Record<SnippetTab, string> {
  const token = i.revealPlaceholder ? PASSWORD_PLACEHOLDER : MASK
  const base = connectStrings(i)
  const uri = base.uri.replace(PASSWORD_PLACEHOLDER, token)
  const envName = i.engine === 'redis' ? 'REDIS_URL' : 'DATABASE_URL'
  return {
    uri,
    env: `${envName}=${uri}`,
    node: nodeSnippet(i.engine, envName),
    python: pythonSnippet(i.engine, envName),
    go: goSnippet(i.engine, envName),
    cli: base.cli,
  }
}

function isMysql(engine: string): boolean {
  return engine === 'mysql' || engine === 'mariadb'
}

function nodeSnippet(engine: string, env: string): string {
  if (engine === 'redis') {
    return `import { createClient } from 'redis'\n\nconst client = createClient({ url: process.env.${env} })\nawait client.connect()`
  }
  if (isMysql(engine)) {
    return `import mysql from 'mysql2/promise'\n\nconst conn = await mysql.createConnection(process.env.${env})`
  }
  return `import pg from 'pg'\n\nconst client = new pg.Client({ connectionString: process.env.${env} })\nawait client.connect()`
}

function pythonSnippet(engine: string, env: string): string {
  if (engine === 'redis') {
    return `import os\nimport redis\n\nr = redis.from_url(os.environ["${env}"])`
  }
  if (isMysql(engine)) {
    return `import os\nfrom urllib.parse import urlparse\nimport pymysql\n\nu = urlparse(os.environ["${env}"])\nconn = pymysql.connect(host=u.hostname, port=u.port, user=u.username, password=u.password)`
  }
  return `import os\nimport psycopg\n\nconn = psycopg.connect(os.environ["${env}"])`
}

function goSnippet(engine: string, env: string): string {
  if (engine === 'redis') {
    return `opt, err := redis.ParseURL(os.Getenv("${env}"))\nif err != nil {\n\tlog.Fatal(err)\n}\nclient := redis.NewClient(opt)`
  }
  if (isMysql(engine)) {
    return `db, err := sql.Open("mysql", os.Getenv("${env}"))\nif err != nil {\n\tlog.Fatal(err)\n}`
  }
  return `conn, err := pgx.Connect(ctx, os.Getenv("${env}"))\nif err != nil {\n\tlog.Fatal(err)\n}`
}
