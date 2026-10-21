import type { CertKind, CertificateStatus } from '../queries/certificates'
import { isInternalCert } from './certStatus'

export type TruthTone = 'success' | 'warning' | 'destructive' | 'info' | 'muted'

export interface CertTruth {
  kind: CertKind
  tone: TruthTone
  daysLeft?: number
  renewalFailed: boolean
  // wantsRealCert is true while visitors would see a browser warning.
  wantsRealCert: boolean
}

// The doctor turns red at this many days; the server reads the same default
// from APP_CERT_FAIL_DAYS, and the badge only needs the rough boundary.
const SOON_DAYS = 14

type TruthInput = Pick<
  CertificateStatus,
  'kind' | 'issuer' | 'source' | 'days_left' | 'status' | 'last_renewal'
>

function kindOf(cert: TruthInput | undefined): CertKind {
  if (!cert) return 'none'
  if (cert.kind) return cert.kind
  if (cert.source === 'custom') return 'custom'
  if (cert.source === 'proxy') return 'proxy'
  return isInternalCert(cert) ? 'internal' : 'other'
}

function toneFor(
  kind: CertKind,
  daysLeft: number | undefined,
  failed: boolean,
): TruthTone {
  if (daysLeft !== undefined && daysLeft < 0) return 'destructive'
  switch (kind) {
    case 'none':
      return 'muted'
    case 'internal':
    case 'staging':
      return 'warning'
    case 'proxy':
      return 'info'
    default:
      if (failed || (daysLeft !== undefined && daysLeft < SOON_DAYS)) {
        return 'destructive'
      }
      return 'success'
  }
}

/** certTruth reduces a stored certificate to what a visitor and an operator need to know. */
export function certTruth(cert: TruthInput | undefined): CertTruth {
  const kind = kindOf(cert)
  const failed = cert?.last_renewal?.result === 'failed'
  const daysLeft = cert?.days_left
  return {
    kind,
    tone: toneFor(kind, daysLeft, failed),
    daysLeft,
    renewalFailed: failed,
    wantsRealCert: kind === 'none' || kind === 'internal' || kind === 'staging',
  }
}
