// Wire shapes for /api/v1/dns (internal/api/dns_zones*.go, internal/dnszones).

export type DnsProviderName = 'cloudflare' | 'route53' | 'googleclouddns'

export type DnsRecordType =
  'A' | 'AAAA' | 'CNAME' | 'TXT' | 'MX' | 'CAA' | 'SRV' | 'NS'

export type DnsRouting = 'simple' | 'weighted' | 'failover' | 'multivalue'

export interface DnsCapabilities {
  proxied: boolean
  routing: boolean
  health_checks: boolean
  apex_cname: boolean
}

export interface DnsZone {
  id: string
  name: string
  provider: string
  status?: string
  record_count?: number
  name_servers: string[]
  private?: boolean
  modified_at?: string
  account_id?: string
  nameservers?: string[]
  delegated?: DnsDelegated
  delegation_reason?: DnsDelegationReason
  current_nameservers?: string[]
}

export type DnsDelegated = 'yes' | 'no' | 'partial' | 'unknown'

export type DnsDelegationReason =
  | 'private_zone'
  | 'lookup_failed'
  | 'no_assigned_nameservers'
  | 'no_public_nameservers'

export interface DnsRegistrarHint {
  name: string
  url?: string
}

/** Detail route: the list view of a zone plus the registrar hint. */
export interface DnsZoneDetail extends DnsZone {
  registrar_hint?: DnsRegistrarHint
  capabilities: DnsCapabilities
}

export interface DnsZonesResponse {
  provider: DnsProviderName | 'none'
  providers: DnsProviderName[]
  capabilities: DnsCapabilities
  capabilities_by_provider?: Record<string, DnsCapabilities>
  provider_errors?: Record<string, string>
  zones: DnsZone[]
}

export interface DnsRecordSet {
  name: string
  type: string
  ttl: number
  values: string[]
  proxied?: boolean
  routing?: string
  set_identifier?: string
  weight?: number
  failover?: 'PRIMARY' | 'SECONDARY' | ''
  health_check_id?: string
  alias?: { dns_name: string; hosted_zone_id: string }
  managed?: boolean
  read_only?: boolean
  managed_by_levelrail?: boolean
}

export interface DnsRecordKey {
  name: string
  type: string
  set_identifier?: string
}

export interface DnsRecordsResponse {
  zone: DnsZone
  provider: string
  capabilities: DnsCapabilities
  records: DnsRecordSet[]
  total: number
  matched?: number
  next_cursor?: string
}

export interface DnsIssue {
  severity: 'error' | 'warning'
  code: string
  message: string
}

export interface DnsRecordWrite {
  record: DnsRecordSet
  issues?: DnsIssue[]
}

export interface DnsZoneOverview {
  zone: DnsZone
  capabilities: DnsCapabilities
  counts: Record<string, number>
  total: number
  last_changed?: string
  last_changed_by?: string
  last_action?: string
}

export type DelegationState =
  'not_delegated' | 'partially_delegated' | 'delegated' | 'delegated_elsewhere'

export interface ResolverDelegation {
  server: string
  name_servers?: string[]
  verdict: 'match' | 'partial' | 'elsewhere' | 'none'
  error?: string
}

export interface DnsDelegationResponse {
  delegation: {
    domain: string
    state: DelegationState
    expected: string[]
    resolvers: ResolverDelegation[]
    elsewhere?: string[]
  }
  checked_at: string
}

export interface DnsChange {
  action: 'create' | 'update' | 'delete' | 'unchanged'
  set: DnsRecordSet
  before?: DnsRecordSet
  issues?: DnsIssue[]
}

export interface DnsPlanResult {
  plan: {
    changes: DnsChange[]
    summary: Partial<Record<DnsChange['action'], number>>
    blocked: boolean
  }
  warnings?: string[]
  errors?: string[]
  applied: number
}

export interface DnsDiscoverResult {
  servers: string[]
  records: DnsRecordSet[]
  plan: DnsPlanResult['plan']
}

export interface DnsTemplateParam {
  key: string
  label: string
  required: boolean
  default?: string
  placeholder?: string
}

export interface DnsTemplate {
  id: string
  name: string
  description: string
  params: DnsTemplateParam[] | null
}

export interface DnsPropagationAnswer {
  server: string
  source: 'resolver' | 'authoritative'
  values?: string[]
  ttl: number
  cname?: string
  error?: string
  rcode?: string
  matches?: boolean
}

export interface DnsPropagation {
  name: string
  type: string
  expected?: string[]
  answers: DnsPropagationAnswer[]
  agree: boolean
}

export interface DnsHealthCheck {
  id?: string
  type: 'HTTP' | 'HTTPS' | 'TCP'
  ip_address?: string
  fqdn?: string
  port?: number
  resource_path?: string
}
