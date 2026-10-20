import { isInternalCert } from './certStatus'
import type { CertificateStatus } from '../queries/certificates'

export type CertLineKind =
  | 'none'
  | 'issuing'
  | 'healthy'
  | 'expiring'
  | 'stalled'
  | 'expired'
  | 'internal'
  | 'custom'
  | 'customExpired'

export type CertLineCert = Pick<
  CertificateStatus,
  'status' | 'renewal' | 'issuer' | 'source' | 'not_after'
>

/** The one sentence kind a certificate reads as, first match wins. */
export function certLineKind(
  cert: CertLineCert | undefined,
  issuing: boolean,
): CertLineKind {
  if (!cert) return issuing ? 'issuing' : 'none'
  if (cert.source === 'custom') {
    return cert.status === 'expired' ? 'customExpired' : 'custom'
  }
  if (cert.status === 'expired') return 'expired'
  if (cert.renewal === 'stalled') return 'stalled'
  if (isInternalCert(cert)) return 'internal'
  return cert.status === 'expiring_soon' ? 'expiring' : 'healthy'
}
