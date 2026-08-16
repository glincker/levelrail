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
