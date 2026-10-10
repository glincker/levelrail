// Wire types for external databases (internal/api/external_databases.go).
// None of these carry a password: it is only ever sent in a request body
// and read back through the explicit reveal route.

export type ExternalEngine =
  'postgres' | 'mysql' | 'mariadb' | 'mongodb' | 'redis'

export type ExternalTlsMode = 'disable' | 'prefer' | 'require'

export type ExternalHealthStatus =
  'reachable' | 'slow' | 'auth_failed' | 'tls_error' | 'unreachable' | 'unknown'

export interface ExternalHealth {
  status: ExternalHealthStatus
  reason?: string
  latency_ms: number
  checked_at?: string
}

export interface ExternalDatabase {
  name: string
  engine: ExternalEngine
  host: string
  port: number
  username?: string
  database?: string
  tls_mode: ExternalTlsMode
  network?: string
  node_id?: string
  project_id?: string
  source_container?: string
  has_password: boolean
  external: boolean
  health?: ExternalHealth
}

export interface ExternalDatabaseRequest {
  name?: string
  engine?: ExternalEngine
  host?: string
  port?: number
  username?: string
  password?: string
  database?: string
  tls_mode?: ExternalTlsMode
  network?: string
  node_id?: string
  container?: string
}

export interface ExternalProbeResult {
  status: ExternalHealthStatus
  reason?: string
  latency_ms: number
  version?: string
  user?: string
  database?: string
  databases?: string[]
}

export interface ExternalCandidate {
  container_id: string
  container: string
  image: string
  engine: ExternalEngine
  port: number
  suggested_host?: string
  network?: string
  networks?: string[]
  published_port?: number
  suggested_user?: string
  note?: string
}
