import type { ProxyDomain, ProxyIntegration } from '../queries/proxyIntegration'

export const POLL_INTERVAL_MS = 4_000
export const MAX_POLLS = 30

export type DomainLiveState = 'todo' | 'handled' | 'waiting' | 'live' | 'error'

// live: route loaded, reachable, valid certificate. waiting: route in place,
// certificate not issued yet. handled: route written, proxy has not answered.
export function domainLiveState(d: ProxyDomain): DomainLiveState {
  if (d.state === 'error' || d.last_error) return 'error'
  if (d.state === 'missing' || d.state === 'stale') return 'todo'
  if (d.reachable && d.certificate?.valid) return 'live'
  if (d.reachable || d.proxy_loaded === true) return 'waiting'
  return 'handled'
}

export function allDomainsLive(domains: ProxyDomain[]): boolean {
  return (
    domains.length > 0 && domains.every((d) => domainLiveState(d) === 'live')
  )
}

// Only domains the proxy can still change on its own are worth polling.
export function shouldPoll(data: ProxyIntegration | null | undefined): boolean {
  if (!data || data.detected.kind === 'none') return false
  return data.domains.some((d) => {
    const s = domainLiveState(d)
    return s === 'handled' || s === 'waiting'
  })
}

export function nextPollInterval(
  data: ProxyIntegration | null | undefined,
  pollsDone: number,
): number | false {
  if (pollsDone >= MAX_POLLS) return false
  return shouldPoll(data) ? POLL_INTERVAL_MS : false
}

export function shouldShowSetupCard(
  data: ProxyIntegration | null | undefined,
): data is ProxyIntegration {
  if (!data || data.detected.kind === 'none') return false
  return !allDomainsLive(data.domains)
}

export function findProxyDomain(
  data: ProxyIntegration | null | undefined,
  domain: string,
): ProxyDomain | undefined {
  return data?.domains.find((d) => d.domain === domain)
}
