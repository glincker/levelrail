import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { certTruth } from '../lib/certTruth'
import type { CertificateStatus } from '../queries/certificates'

function dateOf(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
  })
}

/** CertTruthBadge says which certificate a domain serves, how long it has and how the last renewal went. */
export function CertTruthBadge({
  cert,
  showRenewal = true,
}: {
  cert?: CertificateStatus
  showRenewal?: boolean
}) {
  const { t } = useTranslation('https')
  const truth = certTruth(cert)
  const days = truth.daysLeft
  const expiry =
    days === undefined || truth.kind === 'none' || truth.kind === 'internal'
      ? ''
      : days < 0
        ? t('domainCert.badge.expired')
        : t('domainCert.badge.days', { count: days })
  const renewal = cert?.last_renewal
  return (
    <span className="flex min-w-0 flex-col items-start gap-0.5">
      <Badge
        variant={truth.tone}
        title={t(`domainCert.kindHint.${truth.kind}`)}
        data-testid="cert-truth-badge"
      >
        {t(`domainCert.kind.${truth.kind}`)}
        {expiry ? `, ${expiry}` : ''}
      </Badge>
      {showRenewal && renewal ? (
        <span
          className={
            renewal.result === 'failed'
              ? 'text-xs text-tone-danger'
              : 'text-xs text-muted-foreground'
          }
          title={renewal.error}
        >
          {renewal.result === 'failed'
            ? t('domainCert.badge.renewalFailed', { date: dateOf(renewal.at) })
            : t('domainCert.badge.renewed', { date: dateOf(renewal.at) })}
          {renewal.result === 'failed' && cert?.next_retry_at
            ? ` ${t('domainCert.badge.retrying', { date: dateOf(cert.next_retry_at) })}`
            : ''}
        </span>
      ) : null}
    </span>
  )
}
