export interface CertNotice {
  domain: string
  daysLeft: number
  issuer: string
  reason: string
}

const NOTICE_RE =
  /^(?:- )?(\S+): (expires in (\d+) days?|expires in 1 day|expires today|has expired) \(issuer ([^)]*)\)\. (.*?)\.?$/m

export function parseCertNotice(detail: string | undefined): CertNotice | null {
  const m = NOTICE_RE.exec(detail ?? '')
  if (!m) return null
  const when = m[2] ?? ''
  let daysLeft = Number(m[3])
  if (when === 'expires in 1 day') daysLeft = 1
  if (when === 'expires today') daysLeft = 0
  if (when === 'has expired') daysLeft = -1
  return {
    domain: m[1] ?? '',
    daysLeft,
    issuer: m[4] ?? '',
    reason: m[5] ?? '',
  }
}

export function isDnsReason(reason: string): boolean {
  return /dns|nxdomain|no such host|no valid a|resolve/i.test(reason)
}
