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
