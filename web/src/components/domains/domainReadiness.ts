import type {
  DnsSource,
  DomainCheckResult,
  ProxyCertificate,
} from '../../queries/domainCheck'
import type { ProxyDomain } from '../../queries/proxyIntegration'

export type StepState = 'done' | 'progress' | 'blocked' | 'waiting'
export type ReadinessStepId = 'dns' | 'cert' | 'site'
export type RouteProblem = '503' | '404'

export interface ReadinessInput {
  status: DomainCheckResult['status']
  verdict?: DomainCheckResult['verdict']
  tlsOwner?: DomainCheckResult['tls_owner']
  hasAcmeFailure: boolean
  certTrusted: boolean
  proxyCertificate?: ProxyCertificate | null
  proxyRow?: ProxyDomain
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
  const site: StepState = answering
    ? 'done'
    : !certDone
      ? 'waiting'
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

// Route status only reaches us as text in the proxy row, so look for the code there.
export function routeProblem(row?: ProxyDomain): RouteProblem | undefined {
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
