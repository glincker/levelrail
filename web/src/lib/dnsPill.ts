import type {
  DomainCheckStatus,
  ProxyCertificate,
} from '../queries/domainCheck'
import { tlsSummary } from './proxyIntegration'
import type { ProxyDomain } from '../queries/proxyIntegration'

export type DnsPillState = DomainCheckStatus | 'cert_pending'

// Connected means DNS resolves to this server and, when a proxy owns TLS, its
// certificate is trusted; otherwise the pill must not read green.
export function dnsPillState(
  status: DomainCheckStatus,
  proxyOwned: boolean,
  certTrusted: boolean,
): DnsPillState {
  if (status === 'connected' && proxyOwned && !certTrusted)
    return 'cert_pending'
  return status
}

// One source for the trust question: the proxy diagnosis row when we have it,
// else the certificate the check last saw.
export function proxyCertTrusted(
  row: ProxyDomain | undefined,
  cert: Pick<ProxyCertificate, 'status'> | undefined | null,
): boolean {
  if (row) return tlsSummary(row) === 'ready'
  return cert != null && cert.status !== 'expired'
}

// DOM id of a domain's DNS check card, so the unified status can scroll to it.
export function dnsCheckAnchor(domain: string): string {
  return `dns-check-${domain}`
}
