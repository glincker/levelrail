// Mirrors internal/api/domain_enforcement.go's domainEnforcement.
export type EnforcementMode =
  'levelrail' | 'proxy:traefik' | 'proxy:external' | 'none'

export type EnforcementReason =
  'route_unverified' | 'proxy_unmanaged' | 'not_routed'

export type TrafficControl =
  | 'maintenance'
  | 'basic_auth'
  | 'auth_gate'
  | 'waf'
  | 'redirect'
  | 'error_pages'

export type DomainEnforcement = { mode: EnforcementMode } & Record<
  TrafficControl,
  boolean
> & { reason?: EnforcementReason }

// Fail closed: unknown or missing enforcement never counts as enforced.
export function isEnforced(
  enforcement: DomainEnforcement | undefined,
  control: TrafficControl,
): boolean {
  return enforcement?.[control] === true
}

export const enforcementProxyKey = {
  'proxy:traefik': 'enforcement.proxy.traefik',
  'proxy:external': 'enforcement.proxy.external',
  levelrail: 'enforcement.proxy.external',
  none: 'enforcement.proxy.external',
} as const satisfies Record<EnforcementMode, string>

export const enforcementReasonKey = {
  route_unverified: 'enforcement.reason.route_unverified',
  proxy_unmanaged: 'enforcement.reason.proxy_unmanaged',
  not_routed: 'enforcement.reason.not_routed',
} as const satisfies Record<EnforcementReason, string>

export function isProxyMode(mode: EnforcementMode | undefined): boolean {
  return mode === 'proxy:traefik' || mode === 'proxy:external'
}

export const ENFORCEMENT_DOCS_PATH =
  '/behind-an-existing-proxy#traffic-controls-behind-a-proxy'
