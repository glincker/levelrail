import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleNotchIcon,
  PauseCircleIcon,
  RocketLaunchIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { RelativeTime, SkeletonList, type Tone } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import type { AppListEntry } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import { deployRows, type DeployRow } from '../../lib/dashboardHome'
import { pickSampleApps } from '../../lib/fleetStats'
import { useFleetDeploys } from '../../queries/fleetTraffic'
import { BrandTile } from '../BrandLogoBadge'
import { FLOATING_CARD } from './cardStyle'
import { useScopeName } from './useScopeName'

const ACTIVITY_APP_CAP = 6
const ROWS_SHOWN = 6

const STATUS_VIEW: Record<
  DeployAttempt['status'],
  { tone: Tone; icon: ReactNode }
> = {
  succeeded: { tone: 'success', icon: <CheckCircleIcon weight="fill" /> },
  failed: { tone: 'danger', icon: <XCircleIcon weight="fill" /> },
  running: {
    tone: 'info',
    icon: (
      <CircleNotchIcon className="animate-spin motion-reduce:animate-none" />
    ),
  },
  held: { tone: 'warning', icon: <PauseCircleIcon /> },
  superseded: { tone: 'neutral', icon: <RocketLaunchIcon /> },
  queued: { tone: 'neutral', icon: <PauseCircleIcon /> },
  canceled: { tone: 'neutral', icon: <XCircleIcon /> },
}

function Row({ row, last }: { row: DeployRow; last: boolean }) {
  const { t } = useTranslation('dashboard')
  const view = STATUS_VIEW[row.status]
  return (
    <li className="relative flex gap-3 pb-4 last:pb-0">
      {last ? null : (
        <span
          aria-hidden="true"
          className="absolute top-8 bottom-0 left-4 w-px -translate-x-1/2 bg-border"
        />
      )}
      <BrandTile name={row.brand.name} label={row.brand.label} size="sm" />
      <Link
        to="/apps/$name/deploys"
        params={{ name: row.app }}
        className="-mx-2 -my-1 flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-0.5 rounded-lg px-2 py-1 transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
      >
        <span className="min-w-0 flex-1 basis-40">
          <span className="block truncate text-sm font-medium text-foreground">
            {row.app}
          </span>
          {row.detail ? (
            <span className="block truncate font-mono text-xs text-muted-foreground">
              {row.detail}
            </span>
          ) : null}
        </span>
        <span
          className={cn(
            'inline-flex items-center gap-1 text-xs font-medium [&_svg]:size-4',
            TONE[view.tone].text,
          )}
        >
          <span aria-hidden="true">{view.icon}</span>
          {t(`deploys.status.${row.status}`)}
        </span>
        <span className="text-xs text-muted-foreground">
          <RelativeTime at={row.at} live />
        </span>
      </Link>
    </li>
  )
}

export function RecentActivity({ sample }: { sample: AppListEntry[] }) {
  const { t } = useTranslation('dashboard')
  const names = useMemo(
    () => pickSampleApps(sample, ACTIVITY_APP_CAP),
    [sample],
  )
  const { byApp, isPending } = useFleetDeploys(names)
  const scoped = useScopeName() !== ''
  const rows = deployRows(byApp, ROWS_SHOWN)
  return (
    <section
      aria-label={t('deploys.title')}
      className={cn('space-y-3 p-4 sm:p-5', FLOATING_CARD)}
    >
      <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
        {t('deploys.title')}
      </h2>
      {isPending ? (
        <SkeletonList rows={4} />
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t(
            scoped && sample.length === 0
              ? 'deploys.emptyScoped'
              : 'deploys.empty',
          )}
        </p>
      ) : (
        <ol className="flex flex-col">
          {rows.map((row, i) => (
            <Row key={row.id} row={row} last={i === rows.length - 1} />
          ))}
        </ol>
      )}
    </section>
  )
}
