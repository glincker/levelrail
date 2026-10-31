import { useTranslation } from 'react-i18next'
import { RotateCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { DnsDelegated, DnsDelegationReason } from '@/types/dns'

const TONE: Record<DnsDelegated, string> = {
  yes: 'border-tone-success-border bg-tone-success-soft',
  partial: 'border-tone-warning-border bg-tone-warning-soft',
  no: 'border-tone-warning-border bg-tone-warning-soft',
  unknown: 'border-border bg-muted',
}

/** Whether public resolvers return the zone's assigned name servers, driven by the zone API's delegated field. */
export function ZoneDelegationBanner({
  delegated,
  reason,
  showSteps = false,
  onRecheck,
  checking = false,
}: {
  delegated: DnsDelegated
  reason?: DnsDelegationReason
  showSteps?: boolean
  onRecheck?: () => void
  checking?: boolean
}) {
  const { t } = useTranslation('dnsProviders')
  const message = (() => {
    switch (delegated) {
      case 'yes':
        return t('delegation.bannerDelegated')
      case 'partial':
        return t('delegation.bannerPartial')
      case 'no':
        return t('delegation.bannerNot')
      default:
        return reason
          ? t(`delegation.reason.${reason}`)
          : t('delegation.bannerUnknown')
    }
  })()
  return (
    <div
      role="status"
      className={cn('space-y-2 rounded-lg border p-3 text-sm', TONE[delegated])}
    >
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1">{message}</p>
        {onRecheck ? (
          <Button
            size="sm"
            variant="outline"
            disabled={checking}
            onClick={onRecheck}
          >
            <RotateCwGlassIcon size="1.4em" />
            {checking ? t('delegation.checking') : t('delegation.recheck')}
          </Button>
        ) : null}
      </div>
      {showSteps && (delegated === 'no' || delegated === 'partial') ? (
        <div>
          <p className="text-xs font-medium">{t('delegation.stepsHeading')}</p>
          <ol className="list-decimal pl-5 text-xs text-muted-foreground">
            <li>{t('delegation.step1')}</li>
            <li>{t('delegation.step2')}</li>
            <li>{t('delegation.step3')}</li>
          </ol>
        </div>
      ) : null}
    </div>
  )
}
