import { classifyHost, isPrivateClass } from '../../lib/addressScope'
import type { AppUrlAddress, AppUrls } from '../../queries/appUrls'
import type { ProbeTarget } from '../../queries/appExposure'

export type ReachKind =
  | 'domain'
  | 'public'
  | 'private'
  | 'loopback'
  | 'internal'
  | 'container'
  | 'host'

export type ReachNote =
  | 'local'
  | 'localNone'
  | 'internal'
  | 'domain'
  | 'domainAuto'
  | 'private'
  | 'privateLocalhost'
  | 'privateRemote'
  | 'public'
  | 'container'
  | 'host'
  | 'hostStopped'
  | 'hostNone'

export type ReachScope = 'internet' | 'network' | 'server' | 'apps'
export type ReachSecurity = 'secure' | 'selfSigned' | 'plain' | 'none'

export interface ReachAddress {
  key: string
  kind: ReachKind
  /** What the copy button puts on the clipboard; empty hides the button. */
  value: string
  display: string
  href?: string
  scope: ReachScope
  security: ReachSecurity
  automatic: boolean
  /** Key under appAccess:note.* */
  note: ReachNote
  allowNetwork?: boolean
}

export interface ReachModel {
  running: boolean
  hero: ReachAddress | null
  /** True when the hero is a private address because nothing public exists. */
  promoted: boolean
  /** True when an automatic domain failed its probe and an address that answers took its place. */
  demoted: boolean
  others: ReachAddress[]
  advanced: ReachAddress[]
  autoExpand: boolean
}

const AUTOMATIC_DOMAIN = /\.(sslip\.io|nip\.io|traefik\.me)$/i

function fromAddress(
  a: AppUrlAddress,
  i: number,
  local: boolean,
): ReachAddress {
  const key = `${a.kind}-${a.host ?? ''}-${i}`
  const url = a.url ?? ''
  const base = { key, value: url, display: url, href: a.url, automatic: false }
  if (a.kind === 'loopback') {
    return {
      ...base,
      kind: 'loopback',
      scope: 'server',
      security: 'none',
      note: url ? 'local' : 'localNone',
    }
  }
  if (a.kind === 'internal') {
    const dns = a.host ? `${a.host}:${a.port ?? 0}` : ''
    return {
      ...base,
      kind: 'internal',
      value: dns,
      display: dns,
      href: undefined,
      scope: 'apps',
      security: 'none',
      note: 'internal',
    }
  }
  if (a.kind === 'domain') {
    const host = a.host ?? ''
    const https = url.startsWith('https://')
    const automatic = AUTOMATIC_DOMAIN.test(host)
    return {
      ...base,
      display: url || host,
      kind: 'domain',
      scope: 'internet',
      automatic,
      security: !https ? 'plain' : automatic ? 'selfSigned' : 'secure',
      note: automatic ? 'domainAuto' : 'domain',
    }
  }
  if (!a.host || isPrivateClass(classifyHost(a.host)) || a.kind === 'lan') {
    const noHost = !a.host
    return {
      ...base,
      kind: 'private',
      scope: 'network',
      security: noHost ? 'none' : 'plain',
      note: !noHost
        ? 'private'
        : a.reachable_hint === 'remote_node'
          ? 'privateRemote'
          : 'privateLocalhost',
      allowNetwork: noHost && a.reachable_hint === 'bound_localhost' && local,
    }
  }
  return {
    ...base,
    kind: 'public',
    scope: 'internet',
    security: 'plain',
    note: 'public',
  }
}

/** Groups the app addresses into hero, other public ways in and advanced rows. */
export function deriveReach(
  urls: AppUrls,
  probes: ProbeTarget[] = [],
): ReachModel {
  const all = urls.addresses.map((a, i) => fromAddress(a, i, urls.local_node))
  const withUrl = (k: ReachKind) => all.filter((r) => r.kind === k && r.href)
  const domains = withUrl('domain')
  const publics = withUrl('public')
  const privates = all.filter((r) => r.kind === 'private')
  const custom = domains.find((d) => !d.automatic)
  const firstChoice =
    custom ?? domains.find((d) => d.automatic) ?? publics[0] ?? null
  // A free wildcard-DNS name that does not answer (NAT, no port forward) must
  // not stay first when another address does; a stopped container fails all.
  const containerDown = probes.some((p) => p.kind === 'container' && !p.ok)
  const answers = (r: ReachAddress) =>
    !r.href || !probes.some((p) => p.url.startsWith(r.href ?? '') && !p.ok)
  const alternative =
    firstChoice?.automatic && !answers(firstChoice) && !containerDown
      ? ([...publics, ...privates, ...withUrl('loopback')].find(
          (r) => r.href && answers(r),
        ) ?? null)
      : null
  const hero = alternative ?? firstChoice
  const promotedHero = hero ? null : (privates.find((r) => r.href) ?? null)
  const top = hero ?? promotedHero
  const others = [...domains, ...publics].filter((r) => r !== hero)
  const rest = (k: ReachKind) => all.filter((r) => r.kind === k && r !== top)
  const advanced: ReachAddress[] = [
    ...rest('loopback'),
    ...privates.filter((r) => r !== top),
    {
      key: 'container',
      kind: 'container',
      value: String(urls.container_port),
      display: String(urls.container_port),
      scope: 'apps',
      security: 'none',
      automatic: false,
      note: 'container',
    },
    {
      key: 'host',
      kind: 'host',
      value: urls.host_port ? String(urls.host_port) : '',
      display: urls.host_port ? String(urls.host_port) : '',
      scope: 'server',
      security: 'none',
      automatic: false,
      note: !urls.running
        ? 'hostStopped'
        : urls.host_port
          ? 'host'
          : 'hostNone',
    },
    ...rest('internal'),
  ]
  return {
    running: urls.running,
    hero: top,
    promoted: promotedHero !== null,
    demoted: alternative !== null,
    others,
    advanced,
    autoExpand: hero === null,
  }
}

/** TLS state shown on a lock badge: a probe result beats the address guess. */
export function resolveSecurity(
  a: ReachAddress,
  probe?: ProbeTarget,
): ReachSecurity {
  if (a.security === 'none' || a.security === 'plain') return a.security
  if (probe?.failure === 'tls_not_issued') return 'selfSigned'
  if (probe?.ok) return 'secure'
  return a.security
}

const REASONS = [
  'container_not_running',
  'container_not_listening',
  'firewall_closed',
  'unreachable_from_host',
  'tls_not_issued',
  'proxy_route_missing',
  'upstream_failing',
  'app_error',
] as const

export type ReasonKey = (typeof REASONS)[number] | 'default'

/** Maps a probe failure code to a key under appAccess:test.reason.* */
export function reasonKey(probe: ProbeTarget): ReasonKey {
  return REASONS.find((r) => r === probe.failure) ?? 'default'
}
