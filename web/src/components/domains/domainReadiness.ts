import type {
  DnsSource,
  DomainCheckResult,
  DomainRouteStatus,
  ProxyCertificate,
} from '../../queries/domainCheck'
import type { ProxyDomain } from '../../queries/proxyIntegration'

export type StepState = 'done' | 'progress' | 'blocked' | 'waiting'
export type ReadinessStepId = 'dns' | 'cert' | 'site'
export type RouteProblem = '503' | '404' | '5xx'

export interface ReadinessInput {
  status: DomainCheckResult['status']
  verdict?: DomainCheckResult['verdict']
  tlsOwner?: DomainCheckResult['tls_owner']
  hasAcmeFailure: boolean
  certTrusted: boolean
  proxyCertificate?: ProxyCertificate | null
  proxyRow?: ProxyDomain
  route?: DomainRouteStatus
}

export interface ReadinessStep {
  id: ReadinessStepId
  state: StepState
}

// Public resolvers already right counts as pointed here, a stale local one must not scare.
export function dnsPointsHere(i: ReadinessInput): boolean {
  return (
    i.status === 'connected' ||
    i.verdict === 'dns_ready' ||
    i.verdict === 'public_ready'
  )
}

export function readinessSteps(i: ReadinessInput): ReadinessStep[] {
  const dnsDone = dnsPointsHere(i)
  const dns: StepState = dnsDone
    ? 'done'
    : i.status === 'resolves_elsewhere'
      ? 'blocked'
      : 'progress'
  const ownCert =
    i.tlsOwner === 'custom' ||
    i.tlsOwner === 'internal' ||
    (i.proxyCertificate != null && i.proxyCertificate.status !== 'expired')
  const certDone = i.proxyRow ? i.certTrusted : ownCert || i.certTrusted
  const cert: StepState = certDone
    ? 'done'
    : i.hasAcmeFailure
      ? 'blocked'
      : dnsDone
        ? 'progress'
        : 'waiting'
  const answering = i.proxyRow
    ? i.proxyRow.reachable && certDone
    : dnsDone && certDone
  const routeKind = routeVerdict(i.route)
  const site: StepState = !certDone
    ? 'waiting'
    : routeKind === 'ok'
      ? 'done'
      : routeKind
        ? 'blocked'
        : answering
          ? 'done'
          : i.proxyRow?.last_error
            ? 'blocked'
            : 'progress'
  return [
    { id: 'dns', state: dns },
    { id: 'cert', state: cert },
    { id: 'site', state: site },
  ]
}

export function allStepsDone(steps: ReadinessStep[]): boolean {
  return steps.every((s) => s.state === 'done')
}

// The structured route status wins; undefined means "no usable status".
function routeVerdict(
  route?: DomainRouteStatus,
): RouteProblem | 'ok' | undefined {
  if (!route || route.status <= 0) return undefined
  if (route.status === 503) return '503'
  if (route.status === 404) return '404'
  if (route.status >= 500) return '5xx'
  if (route.status < 400) return 'ok'
  return undefined
}

// Falls back to scanning the proxy row text only when the check has no route field.
export function routeProblem(
  row?: ProxyDomain,
  route?: DomainRouteStatus,
): RouteProblem | undefined {
  const v = routeVerdict(route)
  if (v) return v === 'ok' ? undefined : v
  if (!row || row.reachable) return undefined
  const text = `${row.last_error} ${(row.evidence ?? []).join(' ')}`
  if (/\b503\b/.test(text)) return '503'
  if (/\b404\b/.test(text)) return '404'
  return undefined
}

// Longest authoritative TTL is how long a cache may keep the old answer.
export function longestTtl(sources?: DnsSource[]): number | undefined {
  const ttls = (sources ?? []).map((s) => s.ttl ?? 0).filter((n) => n > 0)
  return ttls.length > 0 ? Math.max(...ttls) : undefined
}

export function ttlLabel(seconds: number): {
  count: number
  unit: 's' | 'm' | 'h'
} {
  if (seconds >= 3600) return { count: Math.round(seconds / 3600), unit: 'h' }
  if (seconds >= 60) return { count: Math.round(seconds / 60), unit: 'm' }
  return { count: seconds, unit: 's' }
}
