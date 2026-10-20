// A leading "*." marks a wildcard, which only DNS-01 can validate.
export const DOMAIN_PATTERN =
  /^(\*\.)?[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i

export function isValidDomain(value: string): boolean {
  return DOMAIN_PATTERN.test(value.trim())
}

export interface WwwPair {
  apex: string
  www: string
  // typed is which side the operator entered.
  typed: 'apex' | 'www'
}

// wwwPair returns the apex/www pair for a typed hostname, or null when the
// hostname is neither an apex nor a www host (a deeper subdomain, or a
// bare label). Two-label domains count as apex; only "www." is stripped.
export function wwwPair(domain: string): WwwPair | null {
  const d = domain.trim().toLowerCase()
  if (!isValidDomain(d)) return null
  const labels = d.split('.')
  if (labels[0] === 'www' && labels.length >= 3) {
    return { apex: labels.slice(1).join('.'), www: d, typed: 'www' }
  }
  if (labels.length === 2) {
    return { apex: d, www: `www.${d}`, typed: 'apex' }
  }
  return null
}

// redirectPair says which hostname redirects to which: the typed one stays
// the canonical site, its counterpart redirects to it.
export function redirectPair(pair: WwwPair): { from: string; to: string } {
  return pair.typed === 'apex'
    ? { from: pair.www, to: pair.apex }
    : { from: pair.apex, to: pair.www }
}

export interface PortSuggestion {
  // port is the port to use, the app's configured one by default.
  port: number
  // switchTo is a different port the container really listens on.
  switchTo?: number
}

// suggestPort prefers the configured port, and only offers a switch when
// the probe saw sockets and none of them is the configured port.
export function suggestPort(
  configured: number,
  listening: number[],
): PortSuggestion {
  if (listening.length === 0 || listening.includes(configured)) {
    return { port: configured }
  }
  return { port: configured, switchTo: listening[0] }
}

export type CertificatePath =
  'http-01' | 'dns-01-private' | 'dns-01-wildcard' | 'upstream-proxy'

// certificatePath mirrors the server verdict, adding the wildcard case so
// the wizard can word it separately from the private-address case.
export function certificatePath(
  domain: string,
  challenge: 'http-01' | 'dns-01-required' | 'upstream-proxy' | undefined,
  privateHost: boolean,
): CertificatePath {
  if (challenge === 'upstream-proxy') return 'upstream-proxy'
  if (domain.trim().startsWith('*.')) return 'dns-01-wildcard'
  if (challenge === 'dns-01-required' || privateHost) return 'dns-01-private'
  return 'http-01'
}
