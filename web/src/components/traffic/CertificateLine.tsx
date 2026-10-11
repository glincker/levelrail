import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  certLineKind,
  type CertLineCert,
  type CertLineKind,
} from '@/lib/trafficCert'
import { daysUntil } from '@/lib/trafficTime'
import { useNow } from '@/hooks/useNow'
import { cn } from '@/lib/utils'
import type { AcmeFailure } from '@/queries/domainCheck'

const RENEWABLE: readonly CertLineKind[] = [
  'healthy',
  'expiring',
  'stalled',
  'expired',
]

export interface CertificateLineProps {
  cert?: CertLineCert
  acmeFailure?: Pick<AcmeFailure, 'action'> | null
  issuing?: boolean
  onRenew?: () => void
  onUseOwn?: () => void
  now?: number
  compact?: boolean
  className?: string
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/** One line that says what the certificate is and what, if anything, to do. */
export function CertificateLine({
  cert,
  acmeFailure,
  issuing = false,
  onRenew,
  onUseOwn,
  now,
  compact = false,
  className,
}: Readonly<CertificateLineProps>) {
  const { t } = useTranslation('traffic')
  const tick = useNow(60_000)
  const current = now ?? tick
  const kind = certLineKind(cert, issuing)
  const date = cert ? formatDate(cert.not_after) : ''
  const days = cert ? Math.max(0, daysUntil(cert.not_after, current) ?? 0) : 0
  const failure = acmeFailure ?? null

  const sentence = t(`cert.${kind}`, { date, count: days })
  const canRenew = onRenew !== undefined && RENEWABLE.includes(kind)
  const ownLabel = kind === 'custom' || kind === 'customExpired'
  const showOwn = onUseOwn !== undefined && kind !== 'internal'

  return (
    <div className={cn('space-y-1', className)}>
      <div
        className={cn(
          'flex flex-wrap items-center gap-x-3 gap-y-1',
          compact ? 'text-xs' : 'text-sm',
        )}
      >
        <span className="font-medium">{t('cert.label')}</span>
        <span
          className={
            kind === 'expired' || kind === 'customExpired'
              ? 'text-destructive'
              : 'text-muted-foreground'
          }
        >
          {sentence}
        </span>
        {canRenew ? (
          <Button type="button" variant="outline" size="xs" onClick={onRenew}>
            {t('cert.renew')}
          </Button>
        ) : null}
        {showOwn ? (
          <Button type="button" variant="ghost" size="xs" onClick={onUseOwn}>
            {ownLabel ? t('cert.replace') : t('cert.useOwn')}
          </Button>
        ) : null}
      </div>
      {failure && kind !== 'custom' ? (
        <p className="text-xs text-muted-foreground">
          {t(`cert.acme.${failure.action}`)}
        </p>
      ) : null}
    </div>
  )
}
