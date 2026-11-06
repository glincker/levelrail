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

export type DomainKind =
  'empty' | 'invalid' | 'wildcard' | 'apex' | 'www' | 'subdomain'

// normalizeDomainInput forgives a pasted URL so a non technical user is not
// blocked by "https://" or a trailing slash.
export function normalizeDomainInput(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/^[a-z]+:\/\//, '')
    .replace(/[/?#].*$/, '')
    .replace(/\.$/, '')
}

export function classifyDomain(value: string): DomainKind {
  const d = normalizeDomainInput(value)
  if (d === '') return 'empty'
  if (d.startsWith('*.')) return 'wildcard'
  if (!isValidDomain(d)) return 'invalid'
  const pair = wwwPair(d)
  if (pair?.typed === 'apex') return 'apex'
  if (pair?.typed === 'www') return 'www'
  return 'subdomain'
}

export type WizardStep = 1 | 2 | 3

export interface WizardState {
  step: WizardStep
  domain: string
  app: string
  // added is the domain already saved to the app, so Back then Next never re-adds it.
  added: string
}

const STORAGE_KEY = 'levelrail.addDomainWizard'

export function loadWizardState(): WizardState | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const v: unknown = JSON.parse(raw)
    if (typeof v !== 'object' || v === null) return null
    const o = v as Record<string, unknown>
    const step = o.step
    if (step !== 1 && step !== 2 && step !== 3) return null
    if (
      typeof o.domain !== 'string' ||
      typeof o.app !== 'string' ||
      typeof o.added !== 'string'
    ) {
      return null
    }
    return { step, domain: o.domain, app: o.app, added: o.added }
  } catch {
    return null
  }
}

export function saveWizardState(state: WizardState): void {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  } catch {
    // Storage can be blocked; the wizard still works, it just forgets on reload.
  }
}

export function clearWizardState(): void {
  try {
    sessionStorage.removeItem(STORAGE_KEY)
  } catch {
    // Nothing to clear when storage is unavailable.
  }
}

export interface DnsRow {
  type: 'A' | 'AAAA' | 'CNAME'
  name: string
  value: string
}

const IPV4 = /^\d{1,3}(\.\d{1,3}){3}$/

// wizardRecords picks the simplest record that works: an apex cannot be a
// CNAME at most registrars, so it gets an A record when an address is known.
export function wizardRecords(
  domain: string,
  kind: DomainKind,
  host: string | undefined,
  ipv4: string | undefined,
  ipv6: string | undefined,
): DnsRow[] {
  if (!host && !ipv4 && !ipv6) return []
  const rows: DnsRow[] = []
  const hostIsIp = host ? host.includes(':') || IPV4.test(host) : false
  if (host && !hostIsIp && kind !== 'apex') {
    return [{ type: 'CNAME', name: domain, value: host }]
  }
  const a = ipv4 ?? (host && IPV4.test(host) ? host : undefined)
  if (a) rows.push({ type: 'A', name: domain, value: a })
  const aaaa = ipv6 ?? (host?.includes(':') ? host : undefined)
  if (aaaa) rows.push({ type: 'AAAA', name: domain, value: aaaa })
  if (rows.length === 0 && host) {
    rows.push({ type: 'CNAME', name: domain, value: host })
  }
  return rows
}
