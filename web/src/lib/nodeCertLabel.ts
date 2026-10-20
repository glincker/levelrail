import type { NodeCertResource } from '../types/nodeCert'

export function nodeCertLabel(cert: NodeCertResource): string {
  switch (cert.state) {
    case 'expired':
      return 'Cert expired'
    case 'revoked':
      return 'Cert revoked'
    case 'unknown':
      return 'Cert expiry unknown'
  }
  const days = cert.days_remaining ?? 0
  return days < 1
    ? 'Cert expires today'
    : `Cert expires in ${days} day${days === 1 ? '' : 's'}`
}
