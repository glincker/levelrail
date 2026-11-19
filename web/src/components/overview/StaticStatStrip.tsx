import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RelativeTime, StatusShape, type StatusState } from '@/components/kit'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { formatSize } from '../../lib/format'
import { useAppVisitors, type VisitorsWindow } from '../../queries/appVisitors'
import { usePagesSiteDetail } from '../../queries/pagesSites'
import type { DeployAttempt } from '../../types/deployAttempt'
import { ERROR_RATE_DANGER } from './config'
import { DEPLOY_STATE, errorShare } from './staticStripModel'

const DASH = '-'
const TILE =
  'flex h-full min-h-[5.25rem] min-w-0 flex-col justify-between gap-1 rounded-lg border border-border bg-card p-3 outline-none focus-visible:ring-2 focus-visible:ring-ring/60'

/** One equal tile: label, value, sub line; the tip is the whole tile's hover. */
function Tile({
  label,
  value,
  sub,
  tip,
  dot,
  danger = false,
  to,
  appName,
}: {
  label: string
  value: ReactNode
  sub?: ReactNode
  tip?: ReactNode
  dot?: StatusState
  danger?: boolean
  to?: '/apps/$name/deploys'
  appName: string
}) {
  const body = (
    <>
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {dot ? <StatusShape state={dot} /> : null}
        <span className="truncate">{label}</span>
      </p>
      <p
        className={cn(
          'truncate text-xl leading-tight font-semibold tabular-nums',
          danger && 'text-destructive',
        )}
      >
        {value}
      </p>
      <p className="min-h-4 truncate text-xs text-muted-foreground">{sub}</p>
    </>
  )
  const inner = to ? (
    <Link to={to} params={{ name: appName }} className={TILE}>
      {body}
    </Link>
  ) : (
    <div tabIndex={tip ? 0 : undefined} className={TILE}>
      {body}
    </div>
  )
  return (
    <li className="min-w-0 max-sm:last:col-span-2">
      {tip ? (
        <Tooltip>
          <TooltipTrigger render={inner} />
          <TooltipContent variant="card" className="block">
            {tip}
          </TooltipContent>
        </Tooltip>
      ) : (
        inner
      )}
    </li>
  )
}

/** Compact bar of the few numbers a static site has; all caveats live in hovers. */
export function StaticStatStrip({
  appName,
  range,
  latest,
}: {
  appName: string
  range: VisitorsWindow
  latest?: DeployAttempt
}) {
  const { t } = useTranslation('appOverview')
  const visitors = useAppVisitors(appName, range).data
  const site = usePagesSiteDetail(appName).data
  const share = visitors ? errorShare(visitors.requests, visitors.status) : null
  const source = latest?.source
  const sha = latest?.commit_sha?.slice(0, 7)

  return (
    <section aria-label={t('staticStrip.label')} className="@container">
      <ul className="grid grid-cols-2 gap-2 @xl:grid-cols-5">
        <Tile
          appName={appName}
          label={t('staticStrip.visitors')}
          value={visitors ? visitors.visitors.toLocaleString() : DASH}
          sub={range}
          tip={t('staticStrip.visitorsInfo')}
        />
        <Tile
          appName={appName}
          label={t('staticStrip.requests')}
          value={visitors ? visitors.requests.toLocaleString() : DASH}
          sub={range}
        />
        <Tile
          appName={appName}
          label={t('staticStrip.errors')}
          value={share === null ? DASH : `${(share * 100).toFixed(1)}%`}
          danger={share !== null && share >= ERROR_RATE_DANGER}
          sub={range}
          tip={
            visitors
              ? t('staticStrip.errorsInfo', {
                  c4: visitors.status['4xx'].toLocaleString(),
                  c5: visitors.status['5xx'].toLocaleString(),
                })
              : t('staticStrip.errorsNone')
          }
        />
        <Tile
          appName={appName}
          to="/apps/$name/deploys"
          label={t('staticStrip.deploy')}
          dot={latest ? DEPLOY_STATE[latest.status] : undefined}
          value={latest ? <RelativeTime at={latest.started_at} live /> : DASH}
          sub={
            latest
              ? (sha ??
                t(`staticLine.source.${source ?? 'other'}`, {
                  defaultValue: t('staticLine.source.other'),
                }))
              : undefined
          }
        />
        <Tile
          appName={appName}
          label={t('staticStrip.size')}
          value={site ? formatSize(site.size_bytes) : DASH}
          sub={
            site && site.file_count > 0
              ? t('staticStrip.files', { count: site.file_count })
              : undefined
          }
        />
      </ul>
    </section>
  )
}
