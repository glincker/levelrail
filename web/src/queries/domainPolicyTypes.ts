// Wire types mirroring internal/trafficpolicy and internal/api/domain_policies*.go.

export type PolicyKind = 'headers' | 'forwarders' | 'geo' | 'cache'

export interface HeaderRule {
  side: 'request' | 'response'
  op: 'set' | 'add' | 'remove'
  name: string
  value?: string
}

export interface SecurityPreset {
  hsts: boolean
  hsts_max_age?: number
  hsts_include_subdomains?: boolean
  content_type_options: boolean
  frame_options?: string
  referrer_policy?: string
  permissions_policy?: string
}

export interface CorsPreset {
  origins: string[]
  methods?: string[]
  headers?: string[]
  expose_headers?: string[]
  credentials?: boolean
  max_age_seconds?: number
  preflight: boolean
}

export interface HeadersSpec {
  rules: HeaderRule[]
  security?: SecurityPreset
  cors?: CorsPreset
  hide_server?: boolean
  forwarded_prefix?: string
}

export interface PathMatch {
  kind: 'prefix' | 'exact' | 'regex'
  path: string
  methods?: string[]
}

export interface Forwarder {
  name?: string
  match: PathMatch
  action: 'app' | 'url' | 'redirect'
  app?: string
  url?: string
  redirect_status?: number
  strip_prefix?: boolean
  rewrite_prefix?: string
  host?: 'preserve' | 'upstream' | 'custom' | ''
  host_value?: string
  websocket?: boolean
  timeout_seconds?: number
}

export interface ForwardersSpec {
  rules: Forwarder[]
}

export interface GeoSpec {
  mode: 'allow' | 'deny'
  countries: string[]
  action: 'block' | 'redirect' | 'error_page'
  redirect_url?: string
  status_code?: number
  body?: string
  exempt?: string[]
  unknown?: 'allow' | 'block'
}

export interface CacheRule {
  match: PathMatch
  ttl_seconds: number
  override_upstream?: boolean
  stale_while_revalidate_seconds?: number
  status_codes?: number[]
  vary?: string[]
  cache_with_cookies?: boolean
}

export interface CacheSpec {
  enabled: boolean
  rules: CacheRule[]
  max_object_bytes?: number
}

export interface RedirectSettings {
  force_https: boolean
  force_https_status?: number
  trailing_slash?: 'off' | 'add' | 'remove' | ''
  lowercase_host?: boolean
}

export interface SpecByKind {
  headers: HeadersSpec
  forwarders: ForwardersSpec
  geo: GeoSpec | null
  cache: CacheSpec
}

export interface PolicyDraft {
  headers?: HeadersSpec
  forwarders?: ForwardersSpec
  geo?: GeoSpec
  cache?: CacheSpec
  redirects?: RedirectSettings
}

export interface FieldError {
  field: string
  message: string
}

export interface PreviewStep {
  stage: string
  outcome: string
  final?: boolean
}

export interface Hop {
  url: string
  status: number
  location?: string
  note: string
}

export interface PreviewHops {
  url: string
  hops: Hop[]
  error?: string
}

export interface PreviewResult {
  steps: PreviewStep[]
  hops?: PreviewHops[]
  errors?: FieldError[]
}

export interface GeoStatus {
  active: boolean
  sources: string[]
  header?: string
  header_needs?: string
  db_path?: string
  db_type?: string
  error?: string
}

export interface CacheBucket {
  minute: number
  hits: number
  misses: number
}

export interface CacheStats {
  hits: number
  misses: number
  bypasses: number
  stale: number
  entries: number
  bytes: number
  series: CacheBucket[]
}

export interface PolicyLimits {
  max_header_rules: number
  max_forwarders: number
  max_cache_rules: number
  max_regex_len: number
  max_header_value_len: number
  cache_max_object_bytes: number
  cache_total_bytes: number
  forward_timeout_seconds: number
}

export interface DomainPolicies {
  domain: string
  app: string
  policy: PolicyDraft
  updated_at: Record<string, string>
  tls_real: boolean
  geo: GeoStatus
  cache: CacheStats
  limits: PolicyLimits
  preview: PreviewStep[]
  warnings?: string[]
  security_preset: SecurityPreset
}

export interface HostRedirect {
  target_url: string
  status_code: number
}

export interface DomainRedirects {
  domain: string
  configured: boolean
  settings: RedirectSettings
  effective: {
    handled_by: 'here' | 'proxy' | 'none'
    explanation: string
    https_port: number
    tls_terminated_upstream: boolean
    ingress_http_redirect: boolean
  }
  app_domains: {
    domain: string
    redirect?: HostRedirect
    maintenance: boolean
    wildcard: boolean
  }[]
  canonical: {
    counterpart?: string
    counterpart_attached: boolean
    counterpart_app?: string
    preset: CanonicalPreset
    dns_hint?: string
    unavailable?: string
  }
  samples: PreviewHops[]
  warnings?: string[]
}

export type CanonicalPreset = 'www-to-apex' | 'apex-to-www' | 'both'

export interface PortStream {
  id: string
  protocol: string
  host_port: number
  container_port: number
  target: string
  open_to_all: boolean
  allowed_sources: string[]
  conflicts: string[]
  holders: { container: string; image: string }[]
}

export interface DomainPorts {
  app: string
  domain: string
  public_https_port: number
  streams: PortStream[]
  detection_note?: string
}

export interface GeoLookup {
  status: GeoStatus
  ip?: string
  country?: string
  source?: string
  private?: boolean
  note?: string
}
