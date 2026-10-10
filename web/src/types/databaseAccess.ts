// Wire shapes for internal/api's database access and network handlers
// (database_access*.go, database_network*.go). No type here holds a secret
// except DatabaseCredential, which the server returns exactly once.

export type AccessPreset = 'read_only' | 'read_write' | 'owner'
export type TempPreset = 'read_only' | 'read_write'
export type DatabaseUserKind = 'user' | 'temp' | 'platform' | 'system'

export interface DatabaseUser {
  name: string
  kind: DatabaseUserKind
  preset?: AccessPreset
  can_login: boolean
  superuser: boolean
  create_db: boolean
  create_role: boolean
  connection_limit: number
  valid_until?: string
  expired: boolean
  connections: number
  protected: boolean
  managed: boolean
  created_by?: string
  created_at?: string
}

export interface DatabaseCredential {
  username: string
  password: string
  database: string
  host: string
  port: number
  sslmode: string
  internal_url: string
  external_url?: string
  external_note?: string
  expires_at?: string
}

export interface CreateDatabaseUserInput {
  name: string
  preset: AccessPreset
  connection_limit?: number
  expires_at?: string
}

export interface CreatedDatabaseUser {
  user: DatabaseUser
  credential: DatabaseCredential
}

export interface DatabaseTemp {
  id: string
  role: string
  preset: TempPreset
  created_by: string
  created_at: string
  expires_at: string
  state: string
}

export interface TempLimits {
  min_minutes: number
  max_minutes: number
  default_minutes: number
}

export interface TempList {
  items: DatabaseTemp[]
  limits: TempLimits
}

export interface IssuedTemp {
  temp: DatabaseTemp
  credential: DatabaseCredential
  clamped: boolean
  limits: TempLimits
}

export interface DatabasePrincipal {
  type: 'user' | 'token'
  id: string
  name: string
  effective: string[]
  via: string[]
}

export interface DatabasePolicyRef {
  id: string
  name: string
  principals: string[]
  scoped_to_database: boolean
}

export interface DatabaseWho {
  database: string
  principals: DatabasePrincipal[]
  policies: DatabasePolicyRef[]
  grant_templates: GrantTemplate[]
  policies_path: string
}

export type GrantTemplate =
  'database-read-only' | 'database-operator' | 'database-owner'

export interface GrantInput {
  template: GrantTemplate
  principal_type: 'user' | 'token'
  principal_id: string
  preview?: boolean
}

export interface GrantResult {
  policy_name: string
  document: unknown
  principal: string
  applied: boolean
  notes: string[]
}

export type NetworkScope = 'platform' | 'project' | 'environment'
export type VerdictLevel =
  'stopped' | 'private' | 'restricted' | 'exposed' | 'unknown'

export interface DatabaseClient {
  app: string
  via: string
  project_id?: string
  project_name?: string
  environment_id?: string
  environment?: string
  in_scope: boolean
  scope_reason?: string
}

export interface DatabaseRule {
  source: string
  description?: string
}

export interface DatabaseNetwork {
  database: string
  running: boolean
  internal: { host: string; address?: string; port: number }
  networks: {
    name: string
    address?: string
    kind: 'default-bridge' | 'app' | 'other'
  }[]
  published?: {
    host_port: number
    container_port: number
    bind: string[]
    class: string
    severity?: string
    explanation?: string
    managed: boolean
  }
  clients: DatabaseClient[]
  verdict: { level: VerdictLevel; text: string; clients: number; port?: number }
  rules: {
    port?: number
    protocol?: string
    active: boolean
    allow: DatabaseRule[]
    missing?: string[]
    extra?: string[]
    can_restrict: boolean
    cannot_restrict_reason?: string
  }
  scope: {
    current: NetworkScope
    project_id?: string
    project_name?: string
    environment_id?: string
    environment?: string
  }
  tls: {
    supported: boolean
    enabled: boolean
    required: boolean
    state?: string
    drift: boolean
  }
  caveats: string[]
}

export interface RulesInput {
  allow: DatabaseRule[]
  local_containers?: boolean
  confirm?: boolean
}

export interface RulesPlan {
  commands: string[]
  drops: string
  tag: string
  persistence: string
  port: number
  protocol: string
  allow: string[]
  warnings: string[]
  applied: boolean
}

export interface ScopeVerdict {
  app: { name: string; project_id?: string; environment_id?: string }
  allowed: boolean
  reason?: string
}

export interface ScopeResult {
  scope: NetworkScope
  applied: boolean
  confirm_required: boolean
  verdicts: ScopeVerdict[]
  lost: ScopeVerdict[]
  notes: string[]
}
