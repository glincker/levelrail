import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { certExpiryDate } from '../lib/certStatus'
import type { ProxyCertificate } from '../queries/domainCheck'

// One coherent line for a hostname whose TLS a fronting proxy owns: what the
// proxy serves, and a way to its page. Replaces ACME and port 80 guidance.
export function ProxyManagedCertNote({
  certificate,
  compact = false,
}: {
  certificate?: Pick<ProxyCertificate, 'issuer' | 'not_after'> | null
  compact?: boolean
}) {
  const { t } = useTranslation('domains')
  const date = certificate ? certExpiryDate(certificate.not_after) : ''
  let text = t('proxyManaged.unknown')
  if (certificate && date) {
    text = certificate.issuer
      ? t('proxyManaged.line', { issuer: certificate.issuer, date })
      : t('proxyManaged.lineNoIssuer', { date })
  }
  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"
    >
      <ShieldCheckGlassIcon
        className="size-4 shrink-0 text-tone-success"
        aria-hidden="true"
      />
      <span>{text}</span>
      {compact ? null : (
        <Link to="/network/proxy" className="underline">
          {t('proxyManaged.link')}
        </Link>
      )}
    </div>
  )
}
