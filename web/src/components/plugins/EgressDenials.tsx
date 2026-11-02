import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatRelativeTime } from '../../lib/relativeTime'
import type { Plugin } from '../../queries/plugins'

const TOP_REASONS = 2

// Hidden in open mode: there is no proxy to deny anything.
export function EgressDenials({ plugin }: Readonly<{ plugin: Plugin }>) {
  const { t } = useTranslation('plugins')
  if (plugin.egress_mode !== 'proxy') return null
  const labels: Record<string, string> = {
    host_not_allowed: t('egress.summary.reasons.host_not_allowed'),
    blocked_address: t('egress.summary.reasons.blocked_address'),
    resolve_failed: t('egress.summary.reasons.resolve_failed'),
    dial_failed: t('egress.summary.reasons.dial_failed'),
    too_many_tunnels: t('egress.summary.reasons.too_many_tunnels'),
    rate_limited: t('egress.summary.reasons.rate_limited'),
    bad_request: t('egress.summary.reasons.bad_request'),
    unknown_caller: t('egress.summary.reasons.unknown_caller'),
  }
  const total = plugin.egress_denials?.total ?? 0
  if (total === 0) {
    return (
      <p className="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
        <CircleCheckGlassIcon className="size-4" aria-hidden="true" />
        {t('egress.summary.none')}
      </p>
    )
  }
  const reasons = Object.entries(plugin.egress_denials?.by_reason ?? {})
    .filter(([, n]) => n > 0)
    .sort((a, b) => b[1] - a[1])
    .slice(0, TOP_REASONS)
    .map(([key, n]) => `${labels[key] ?? key.replaceAll('_', ' ')} (${n})`)
  const last = plugin.egress_denials?.last_denied
  const when = last ? formatRelativeTime(last) : ''
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <p
            tabIndex={0}
            className="mt-1 flex flex-wrap items-center gap-x-1 text-xs text-warning-foreground"
          >
            <TriangleAlertGlassIcon className="size-4" aria-hidden="true" />
            <span className="font-medium">
              {t('egress.summary.count', { count: total })}
            </span>
            {reasons.length > 0 ? <span>{reasons.join(', ')}</span> : null}
            {when ? (
              <span className="text-muted-foreground">
                {t('egress.summary.last', { when })}
              </span>
            ) : null}
          </p>
        }
      />
      <TooltipContent>{t('egress.summary.help')}</TooltipContent>
    </Tooltip>
  )
}
