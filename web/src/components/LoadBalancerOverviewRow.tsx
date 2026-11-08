import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  ChevronRightGlassIcon,
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { StatusBadge } from '@/components/ui/status-badge'
import {
  type LoadBalancerOverviewState,
  type LoadBalancerSummary,
} from '../queries/loadBalancers'

export const LB_LIST_GRID =
  'grid grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_1rem] items-center gap-3'

const STATE_META = {
  balancing: {
    variant: 'success',
    label: 'lb.overview.stateBalancing',
    icon: CircleCheckGlassIcon,
  },
  degraded: {
    variant: 'warning',
    label: 'lb.overview.stateDegraded',
    icon: TriangleAlertGlassIcon,
  },
  none: {
    variant: 'muted',
    label: 'lb.overview.stateNone',
    icon: CircleMinusGlassIcon,
  },
} as const satisfies Record<
  LoadBalancerOverviewState,
  {
    variant: 'success' | 'warning' | 'muted'
    label:
      | 'lb.overview.stateBalancing'
      | 'lb.overview.stateDegraded'
      | 'lb.overview.stateNone'
    icon: typeof CircleCheckGlassIcon
  }
>

function lastCheckLabel(
  iso: string | undefined,
  t: TFunction<'statePlaceholders'>,
): string {
  if (!iso) return t('lb.overview.notYet')
  const secs = Math.max(0, Math.round((Date.now() - Date.parse(iso)) / 1000))
  if (secs < 60) return t('lb.overview.secondsAgo', { count: secs })
  if (secs < 3600)
    return t('lb.overview.minutesAgo', { count: Math.round(secs / 60) })
  return t('lb.overview.hoursAgo', { count: Math.round(secs / 3600) })
}

export function LoadBalancerOverviewHeader() {
  const { t } = useTranslation('statePlaceholders')
  return (
    <div
      className={`${LB_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium text-muted-foreground`}
    >
      <span>{t('lb.overview.app')}</span>
      <span>{t('lb.overview.algorithm')}</span>
      <span>{t('lb.overview.state')}</span>
      <span>{t('lb.overview.upstreams')}</span>
      <span>{t('lb.overview.lastCheck')}</span>
      <span aria-hidden="true" />
    </div>
  )
}

export function LoadBalancerOverviewRow({
  item,
}: {
  item: LoadBalancerSummary
}) {
  const { t } = useTranslation('statePlaceholders')
  const meta = STATE_META[item.state]
  return (
    <div
      className={`${LB_LIST_GRID} relative h-full w-full border-b border-border px-4 py-3 text-sm transition-colors hover:bg-muted/60`}
    >
      <Link
        to="/apps/$name/loadbalancer"
        params={{ name: item.app }}
        className="truncate font-medium text-foreground after:absolute after:inset-0"
      >
        {item.app}
      </Link>
      <span className="truncate font-mono text-xs text-muted-foreground">
        {item.algorithm}
      </span>
      <span>
        <StatusBadge
          variant={meta.variant}
          label={t(meta.label)}
          icon={meta.icon}
        />
      </span>
      <span className="text-muted-foreground">
        {item.upstreams_total === 0
          ? t('lb.overview.noneObserved')
          : t('lb.overview.healthyOf', {
              healthy: item.upstreams_healthy,
              total: item.upstreams_total,
            })}
      </span>
      <span className="text-muted-foreground">
        {lastCheckLabel(item.last_check, t)}
      </span>
      <ChevronRightGlassIcon
        className="size-5 text-muted-foreground"
        aria-hidden="true"
      />
    </div>
  )
}
