// Wire types for the integrations hub read model (GET /api/v1/connections).

export type ConnectionKind =
  'git' | 'registry' | 'notify' | 'secret' | 'dns' | 'cloud' | 'webhook'

export type ConnectionAuthMethod =
  'app_install' | 'oauth' | 'token' | 'workload_identity' | 'url_secret'

export type ConnectionScopeType = 'instance' | 'org' | 'app'

export type ConnectionHealth =
  'ok' | 'degraded' | 'needs_reconnect' | 'expired' | 'unknown'

export interface Connection {
  ref: string
  kind: ConnectionKind
  provider: string
  label: string
  account?: string
  host?: string
  auth_method: ConnectionAuthMethod
  capabilities: string[] | null
  bound_count: number
  created_at?: string
  is_default?: boolean
  scope_type: ConnectionScopeType
  scope_id?: string
  health: ConnectionHealth
  health_detail?: string
  checked_at?: string
  last_used_at?: string
  expires_at?: string
  rotate_after?: string
  expiring_soon: boolean
}

export type ConnectMethod =
  'one_click' | 'guided_oauth' | 'token' | 'settings_page'

export interface CatalogEntry {
  id: string
  category: string
  name: string
  connect_methods: ConnectMethod[] | null
  create_endpoint?: string
}

export interface ConnectionListResponse {
  connections: Connection[] | null
}

export interface ConnectionCatalogResponse {
  catalog: CatalogEntry[] | null
}
