import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsInLineHorizontalIcon,
  CalendarXIcon,
  CheckCircleIcon,
  CircleDashedIcon,
  CircleNotchIcon,
  ClockIcon,
  HourglassIcon,
  PauseCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  DOMAIN_STATUS_VARIANT,
  type DomainStatus,
  type DomainStatusReason,
} from '@/lib/trafficStatus'

const STATUS_ICON: Record<DomainStatus, Icon> = {
  live: CheckCircleIcon,
  going_live: CircleNotchIcon,
  propagating: ClockIcon,
  waiting_for_dns: HourglassIcon,
  handled_by_proxy: ArrowsInLineHorizontalIcon,
  needs_attention: WarningIcon,
  expiring_soon: CalendarXIcon,
  paused: PauseCircleIcon,
  not_set_up: CircleDashedIcon,
}

export interface DomainStatusBadgeProps {
  status: DomainStatus | 'loading'
  reason?: DomainStatusReason
  size?: 'sm' | 'md'
  className?: string
}

/** The one status pill every domain surface renders, icon plus text. */
export function DomainStatusBadge({
  status,
  reason,
  size = 'md',
  className,
}: Readonly<DomainStatusBadgeProps>) {
  const { t } = useTranslation('traffic')
  const describedBy = useId()

  if (status === 'loading') {
    return (
      <Skeleton
        role="status"
        aria-label={t('status.loading')}
        className={cn(size === 'sm' ? 'h-5 w-16' : 'h-6 w-20', className)}
      />
    )
  }

  const StatusIcon = STATUS_ICON[status]
  const reasonText = reason ? t(`status.reason.${reason}`) : undefined
  return (
    <>
      <Badge
        variant={DOMAIN_STATUS_VARIANT[status]}
        data-status={status}
        title={reasonText}
        aria-describedby={reasonText ? describedBy : undefined}
        className={cn(size === 'sm' && 'px-1.5 text-[11px]', className)}
      >
        <StatusIcon
          aria-hidden="true"
          className={cn(
            status === 'going_live' &&
              'animate-spin motion-reduce:animate-none',
          )}
        />
        {t(`status.label.${status}`)}
      </Badge>
      {reasonText ? (
        <span id={describedBy} className="sr-only">
          {reasonText}
        </span>
      ) : null}
    </>
  )
}
