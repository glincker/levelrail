import { useTranslation } from 'react-i18next'
import { RotateCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { useDnsDelegation } from '@/queries/dns'
import type { DelegationState } from '@/types/dns'

const TONE: Record<DelegationState | 'unknown', string> = {
  delegated: 'border-tone-success-border bg-tone-success-soft',
  partially_delegated: 'border-tone-warning-border bg-tone-warning-soft',
  delegated_elsewhere: 'border-tone-danger-border bg-tone-danger-soft',
  not_delegated: 'border-tone-warning-border bg-tone-warning-soft',
  unknown: 'border-border bg-muted',
}

/** Whether the world resolves this domain to the zone's name servers; shared by the zone sheet, the wizard and domain pages. */
export function ZoneDelegationBanner({
  zone,
  showSteps = false,
}: {
  zone: string
  showSteps?: boolean
}) {
  const { t } = useTranslation('dnsProviders')
  const { data, isPending, isFetching, error, refetch } = useDnsDelegation(zone)
  if (isPending) return <Skeleton className="h-14 w-full" />
  const d = data?.delegation
  const state: DelegationState | 'unknown' = error || !d ? 'unknown' : d.state
  const message = (() => {
    switch (state) {
      case 'delegated':
        return t('delegation.bannerDelegated')
      case 'partially_delegated':
        return t('delegation.bannerPartial')
      case 'delegated_elsewhere':
        return t('delegation.bannerElsewhere', {
          servers: (d?.elsewhere ?? []).join(', '),
        })
      case 'not_delegated':
        return t('delegation.bannerNot')
      default:
        return t('delegation.bannerUnknown')
    }
  })()
  return (
    <div
      role="status"
      className={cn('space-y-2 rounded-lg border p-3 text-sm', TONE[state])}
    >
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1">{message}</p>
        <Button
          size="sm"
          variant="outline"
          disabled={isFetching}
          onClick={() => void refetch()}
        >
          <RotateCwGlassIcon size="1.4em" />
          {isFetching ? t('delegation.checking') : t('delegation.recheck')}
        </Button>
      </div>
      {showSteps && state !== 'delegated' ? (
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
