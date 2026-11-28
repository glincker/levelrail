import type { LiveState } from '../queries/goLiveChecklist'

export type UrlState =
  'reachable' | 'down' | 'waiting_dns' | 'local_only' | 'unknown'

export type UrlReason =
  | 'ok'
  | 'no_url'
  | 'no_data'
  | 'stopped'
  | 'not_running'
  | 'not_ready'
  | 'waiting_dns'
  | 'local_host'

export interface UrlReachability {
  state: UrlState
  reason: UrlReason
}

export interface ReachabilityNetwork {
  running: boolean
  fallback_url?: string
}

export interface ReachabilityStatus {
  url: string | null
  /** Stuck report ready flag; true when the report is not loaded yet. */
  ready: boolean
  liveState?: LiveState
}

function ipv4Octets(host: string): number[] | null {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host)
  return m ? m.slice(1).map(Number) : null
}

/** True for hosts that only resolve or route on the computer or LAN running Levelrail. */
export function isPrivateHost(rawHost: string): boolean {
  const host = rawHost.toLowerCase().replace(/^\[|\]$/g, '')
  if (host === 'localhost' || host.endsWith('.localhost')) return true
  if (host.endsWith('.local')) return true
  if (host === '::1' || host === '::' || host.startsWith('fe80:')) return true
  if (/^f[cd][0-9a-f]{2}:/.test(host)) return true
  const o = ipv4Octets(host)
  if (!o) return false
  const a = o[0] ?? 0
  const b = o[1] ?? 0
  return (
    a === 127 ||
    a === 0 ||
    a === 10 ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 168) ||
    (a === 169 && b === 254)
  )
}

export function urlHost(url: string): string | null {
  try {
    return new URL(url).hostname
  } catch {
    return null
  }
}

export function isPrivateUrl(url: string): boolean {
  const host = urlHost(url)
  return host !== null && isPrivateHost(host)
}

/** Whether an address can really be opened from another device, so a QR or a live link is honest. */
export function urlReachability(
  app: { suspended?: boolean },
  network: ReachabilityNetwork | undefined,
  status: ReachabilityStatus,
): UrlReachability {
  if (!status.url) return { state: 'unknown', reason: 'no_url' }
  if (app.suspended) return { state: 'down', reason: 'stopped' }
  if (!network) return { state: 'unknown', reason: 'no_data' }
  if (!network.running) return { state: 'down', reason: 'not_running' }
  if (!status.ready) return { state: 'down', reason: 'not_ready' }
  if (isPrivateUrl(status.url)) {
    return { state: 'local_only', reason: 'local_host' }
  }
  if (status.liveState === 'waiting_dns') {
    return { state: 'waiting_dns', reason: 'waiting_dns' }
  }
  return { state: 'reachable', reason: 'ok' }
}

/** A public fallback address the instance reports for a local-only app, if any. */
export function shareableUrl(
  url: string | null,
  reach: UrlReachability,
  network: ReachabilityNetwork | undefined,
): string | null {
  if (!url) return null
  if (reach.state === 'reachable') return url
  const alt = network?.fallback_url
  if (reach.state === 'local_only' && alt && !isPrivateUrl(alt)) return alt
  return null
}
