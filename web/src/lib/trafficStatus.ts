import type { CertificateStatus } from '../queries/certificates'
import type { AcmeFailure, DomainCheckStatus } from '../queries/domainCheck'
import { isInternalCert } from './certStatus'

export const DOMAIN_STATUSES = [
  'live',
  'going_live',
  'propagating',
  'waiting_for_dns',
  'handled_by_proxy',
  'needs_attention',
  'expiring_soon',
  'paused',
  'not_set_up',
] as const

export type DomainStatus = (typeof DOMAIN_STATUSES)[number]

export type DomainStatusVariant =
  'success' | 'info' | 'warning' | 'destructive' | 'muted'

export type DomainStatusAction =
  | 'open_site'
  | 'view_progress'
  | 'check_again'
  | 'show_record'
  | 'show_proxy_setup'
  | 'fix'
  | 'renew'
  | 'turn_off'
  | 'set_up'

// Keys under traffic:status.reason.* in the locale file.
export type DomainStatusReason =
  | 'runInProgress'
  | 'maintenance'
  | 'certExpired'
  | 'renewalStalled'
  | 'acmeFailure'
  | 'resolvesElsewhere'
  | 'routeError'
  | 'externalProxy'
  | 'routeMissing'
  | 'unconfigured'
  | 'noCheck'
  | 'notResolving'
  | 'propagating'
  | 'gettingCertificate'
  | 'certExpiring'
  | 'live'
  | 'livePrivate'

export interface DomainStatusInput {
  runInProgress?: boolean
  maintenanceEnabled?: boolean
  proxyMode?: 'managed' | 'external'
  routeManaged?: boolean
  routeState?: 'ok' | 'error' | 'missing'
  check?: DomainCheckStatus
  expectedPrivate?: boolean
  cert?: Pick<
    CertificateStatus,
    'status' | 'renewal' | 'issuer' | 'source' | 'acme_failure'
  > | null
  acmeFailure?: Pick<AcmeFailure, 'action'> | null
  firstIssuancePending?: boolean
}

export interface DerivedDomainStatus {
  status: DomainStatus
  reason: DomainStatusReason
  action: DomainStatusAction
}

export const DOMAIN_STATUS_VARIANT: Record<DomainStatus, DomainStatusVariant> =
  {
    live: 'success',
    going_live: 'info',
    propagating: 'info',
    waiting_for_dns: 'warning',
    handled_by_proxy: 'muted',
    needs_attention: 'destructive',
    expiring_soon: 'warning',
    paused: 'muted',
    not_set_up: 'muted',
  }

function result(
  status: DomainStatus,
  reason: DomainStatusReason,
  action: DomainStatusAction,
): DerivedDomainStatus {
  return { status, reason, action }
}

// A failed attempt that only needs time (rate limit) is not a failure to fix.
function blockingFailure(input: DomainStatusInput): boolean {
  const failure = input.acmeFailure ?? input.cert?.acme_failure
  return Boolean(failure) && failure?.action !== 'wait_rate_limit'
}

function attention(input: DomainStatusInput): DerivedDomainStatus | null {
  if (input.cert?.status === 'expired') {
    return result('needs_attention', 'certExpired', 'fix')
  }
  if (input.cert?.renewal === 'stalled') {
    return result('needs_attention', 'renewalStalled', 'fix')
  }
  if (blockingFailure(input)) {
    return result('needs_attention', 'acmeFailure', 'fix')
  }
  if (input.check === 'resolves_elsewhere' && input.proxyMode !== 'external') {
    return result('needs_attention', 'resolvesElsewhere', 'fix')
  }
  if (input.routeState === 'error') {
    return result('needs_attention', 'routeError', 'fix')
  }
  return null
}

/**
 * Single mapping from backend wire states to the one status a domain shows.
 * First match wins, in the order of the redesign spec table (section 3.1).
 */
export function deriveDomainStatus(
  input: DomainStatusInput,
): DerivedDomainStatus {
  if (input.runInProgress) {
    return result('going_live', 'runInProgress', 'view_progress')
  }
  if (input.maintenanceEnabled) {
    return result('paused', 'maintenance', 'turn_off')
  }
  const failing = attention(input)
  if (failing) return failing
  if (input.proxyMode === 'external' && input.routeManaged !== true) {
    return result('handled_by_proxy', 'externalProxy', 'show_proxy_setup')
  }
  if (input.check === 'unconfigured') {
    return result('not_set_up', 'unconfigured', 'set_up')
  }
  if (input.routeState === 'missing') {
    return result('not_set_up', 'routeMissing', 'set_up')
  }
  if (input.check === 'not_resolving') {
    return result('waiting_for_dns', 'notResolving', 'show_record')
  }
  if (input.check === 'propagating') {
    return result('propagating', 'propagating', 'check_again')
  }
  if (input.check !== 'connected') {
    return result('not_set_up', 'noCheck', 'set_up')
  }
  const cert = input.cert
  if (!cert || input.firstIssuancePending) {
    return result('going_live', 'gettingCertificate', 'view_progress')
  }
  const internal = isInternalCert(cert)
  if (cert.status === 'expiring_soon' && !internal) {
    return result('expiring_soon', 'certExpiring', 'renew')
  }
  const reason = internal && input.expectedPrivate ? 'livePrivate' : 'live'
  return result('live', reason, 'open_site')
}
