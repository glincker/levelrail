import type { CertificateStatus } from '../queries/certificates'

// Shared between DomainRow.tsx (centralized domains list) and
// DomainEditor.tsx (per-app guided add flow), both render the same
// certificateStatus.status as a Badge.
export const certStatusMeta: Record<
  CertificateStatus['status'],
  { label: string; variant: 'success' | 'warning' | 'destructive' }
> = {
  healthy: { label: 'Healthy', variant: 'success' },
  expiring_soon: { label: 'Expiring soon', variant: 'warning' },
  expired: { label: 'Expired', variant: 'destructive' },
}

// isInternalCert is true for a certificate Caddy's built-in CA signed: it
// lives about half a day and Caddy renews it unattended, so its expiry is
// never a warning. Browsers do not trust it until HTTPS is enabled.
export function isInternalCert(
  cert: Pick<CertificateStatus, 'issuer' | 'source'>,
): boolean {
  return (
    cert.source === 'acme' &&
    /local authority|internal/i.test(cert.issuer ?? '')
  )
}

export const CERT_RENEWAL_STALLED_HINT =
  'Renewal appears stalled: this certificate is expired, or has been expiring with no renewal for hours. Check that the domain resolves to this server and that ports 80 and 443 are reachable, then look at the ingress logs.'

// certRenewalBadge returns badge props only for a stalled renewal, so a
// healthy certificate keeps its single status badge.
export function certRenewalBadge(
  cert: Pick<CertificateStatus, 'renewal' | 'acme_failure'>,
): { label: string; hint: string } | null {
  if (cert.renewal !== 'stalled') {
    return null
  }
  const reason = cert.acme_failure?.error
  return {
    label: 'Renewal stalled',
    hint: reason
      ? `${CERT_RENEWAL_STALLED_HINT} The certificate authority said: ${reason}`
      : CERT_RENEWAL_STALLED_HINT,
  }
}

// certAttentionRank orders certificates for the "needs attention first"
// view on the domains page: stalled renewals ahead of any other
// non-healthy status, healthy or missing certs last.
export function certAttentionRank(cert?: CertificateStatus): number {
  if (!cert || cert.status === 'healthy') {
    return 2
  }
  if (isInternalCert(cert) && cert.status !== 'expired') {
    return 2
  }
  return certRenewalBadge(cert) ? 0 : 1
}

function compareCertAttention(
  a: CertificateStatus | undefined,
  b: CertificateStatus | undefined,
): number {
  const rankA = certAttentionRank(a)
  const rankB = certAttentionRank(b)
  if (rankA !== rankB) {
    return rankA - rankB
  }
  return rankA === 2
    ? 0
    : new Date(a!.not_after).getTime() - new Date(b!.not_after).getTime()
}

// sortByCertAttention puts stalled-renewal items first, then non-healthy
// items by soonest expiry, leaving healthy/uncertified items in their
// existing relative order (stable sort).
export function sortByCertAttention<T>(
  items: T[],
  certFor: (item: T) => CertificateStatus | undefined,
): T[] {
  return [...items].sort((a, b) => compareCertAttention(certFor(a), certFor(b)))
}

const DAY_MS = 24 * 60 * 60 * 1000

export function certExpiryLabel(notAfter: string, now = new Date()): string {
  const end = new Date(notAfter).getTime()
  if (Number.isNaN(end)) {
    return ''
  }
  const diff = end - now.getTime()
  const days = Math.floor(Math.abs(diff) / DAY_MS)
  const unit = days === 1 ? 'day' : 'days'
  if (diff < 0) {
    return days === 0 ? 'expired today' : `expired ${days} ${unit} ago`
  }
  return days === 0 ? 'expires today' : `expires in ${days} ${unit}`
}

const PROXY_CERT_WARN_DAYS = 30

// proxyCertStatus turns the certificate a fronting proxy was seen serving
// into the shape the domain list renders. Untrusted ones (a proxy's default
// certificate) return undefined so they never read as healthy.
export function proxyCertStatus(
  domain: string,
  cert: { issuer: string; not_after: string; valid: boolean },
  now = new Date(),
): CertificateStatus | undefined {
  const end = new Date(cert.not_after).getTime()
  if (!cert.valid || Number.isNaN(end)) {
    return undefined
  }
  const left = end - now.getTime()
  let status: CertificateStatus['status'] = 'healthy'
  if (left < 0) {
    status = 'expired'
  } else if (left < PROXY_CERT_WARN_DAYS * DAY_MS) {
    status = 'expiring_soon'
  }
  return {
    domain,
    issuer: cert.issuer,
    not_before: '',
    not_after: cert.not_after,
    status,
    renewal: 'ok',
    source: 'acme',
    managed_by: 'proxy',
  }
}

export function certExpiryDate(notAfter: string): string {
  const d = new Date(notAfter)
  if (Number.isNaN(d.getTime())) {
    return ''
  }
  return d.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
