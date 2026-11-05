export const TRAFFIC_DROP_KIND = 'domain_traffic_drop'
export const PARKED_CERT_KIND = 'parked_domain_cert_expiry'

export const TRAFFIC_DROP_DEFAULTS = {
  window: '1h',
  percent: 50,
  minRequests: 20,
} as const
export const PARKED_CERT_DEFAULT_DAYS = 14

const MAX_WINDOW_SECONDS = 24 * 3600

const UNIT_SECONDS: Record<string, number> = {
  ns: 1e-9,
  us: 1e-6,
  µs: 1e-6,
  ms: 1e-3,
  s: 1,
  m: 60,
  h: 3600,
}

// Returns null when the text is not a Go-style duration.
export function durationSeconds(text: string): number | null {
  const value = text.trim()
  if (!/^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/.test(value)) return null
  let total = 0
  for (const m of value.matchAll(/(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g)) {
    total += Number(m[1]) * (UNIT_SECONDS[m[2] ?? 's'] ?? 1)
  }
  return total
}

export function isTrafficWindowValid(text: string): boolean {
  const secs = durationSeconds(text)
  return secs !== null && secs <= MAX_WINDOW_SECONDS
}

export function trafficResourceId(appName: string, domain: string): string {
  const name = domain.trim()
  return name ? `domain:${name}` : `service:${appName}`
}

// '' means every domain of the app.
export function domainFromResourceId(resourceId: string | undefined): string {
  return resourceId?.startsWith('domain:') ? resourceId.slice(7) : ''
}

export function isTrafficKind(kind: string): boolean {
  return kind === TRAFFIC_DROP_KIND || kind === PARKED_CERT_KIND
}
