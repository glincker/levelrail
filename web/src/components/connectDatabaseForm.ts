import type {
  ExternalCandidate,
  ExternalEngine,
  ExternalTlsMode,
} from '../types/externalDatabase'

export const ENGINES: ExternalEngine[] = [
  'postgres',
  'mysql',
  'mariadb',
  'mongodb',
  'redis',
]
export const TLS_MODES: ExternalTlsMode[] = ['disable', 'prefer', 'require']
export const TLS_LABEL = {
  disable: 'external.connect.fields.tlsDisable',
  prefer: 'external.connect.fields.tlsPrefer',
  require: 'external.connect.fields.tlsRequire',
} as const

export const DEFAULT_PORT: Record<ExternalEngine, number> = {
  postgres: 5432,
  mysql: 3306,
  mariadb: 3306,
  mongodb: 27017,
  redis: 6379,
}

export function defaultTls(engine: ExternalEngine): ExternalTlsMode {
  return engine === 'mongodb' || engine === 'redis' ? 'disable' : 'prefer'
}

export function slug(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
}

export type Step = 'source' | 'pick' | 'details'

export interface FormState {
  name: string
  engine: ExternalEngine
  host: string
  port: string
  network: string
  username: string
  password: string
  database: string
  tls: ExternalTlsMode
  container: string
}

export const EMPTY_FORM: FormState = {
  name: '',
  engine: 'postgres',
  host: '',
  port: String(DEFAULT_PORT.postgres),
  network: '',
  username: '',
  password: '',
  database: '',
  tls: 'prefer',
  container: '',
}

export function formFromCandidate(c: ExternalCandidate): FormState {
  return {
    ...EMPTY_FORM,
    name: slug(c.container),
    engine: c.engine,
    host: c.suggested_host ?? '',
    port: String(c.port),
    network: c.network ?? '',
    username: c.suggested_user ?? '',
    tls: defaultTls(c.engine),
    container: c.container,
  }
}
