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

export const CERT_RENEWAL_STALLED_HINT =
  'Renewal appears stalled: this certificate is expired, or has been expiring with no renewal for hours. Check that the domain resolves to this server and that ports 80 and 443 are reachable, then look at the ingress logs.'

// certRenewalBadge returns badge props only for a stalled renewal, so a
// healthy certificate keeps its single status badge.
export function certRenewalBadge(
  cert: Pick<CertificateStatus, 'renewal'>,
): { label: string; hint: string } | null {
  return cert.renewal === 'stalled'
    ? { label: 'Renewal stalled', hint: CERT_RENEWAL_STALLED_HINT }
    : null
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
