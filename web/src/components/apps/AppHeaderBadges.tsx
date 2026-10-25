import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ClockCounterClockwiseIcon,
  HardDrivesIcon,
  MoonIcon,
  PlugsConnectedIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { ReactNode } from 'react'
import { RelativeTime } from '../kit/RelativeTime'
import { useQuery } from '@tanstack/react-query'
import { appSleepQueryOptions } from '../../queries/appSleep'
import { AsleepFor } from '../AsleepFor'
import { useAppNetwork } from '../../queries/appNetwork'
import { useDeployAttempts } from '../../queries/deployAttempts'
import type { AppDetail } from '../../types/appDetail'

// Below sm only the icon shows; the text stays reachable via title and sr-only.
function Chip({
  icon,
  children,
  label,
  variant = 'muted',
}: {
  icon: ReactNode
  children: ReactNode
  label: string
  variant?: 'muted' | 'destructive'
}) {
  return (
    <Badge variant={variant} title={label}>
      {icon}
      <span className="max-sm:sr-only">{children}</span>
    </Badge>
  )
}

/** Compact facts for the top right of every app page; each links to its detail. */
export function AppHeaderBadges({
  app,
  attentionCount = 0,
}: {
  app: Pick<AppDetail, 'name' | 'node_id' | 'node_name'>
  attentionCount?: number
}) {
  const { t } = useTranslation('appAccess')
  const { t: tSleep } = useTranslation('sleepHistory')
  const { data: sleep } = useQuery({
    ...appSleepQueryOptions(app.name),
    staleTime: 15_000,
    retry: false,
  })
  const asleepSince = sleep?.sleeping ? sleep.since : undefined
  const { data: network } = useAppNetwork(app.name)
  const { data: attempts } = useDeployAttempts(app.name)
  const lastOk = attempts?.find((a) => a.status === 'succeeded')
  const portLabel = network?.container_port
    ? t('badges.port', { port: network.container_port })
    : ''
  const deployedLabel = t('badges.deployed')
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {attentionCount > 0 ? (
        <a href="#overview-attention">
          <Chip
            variant="destructive"
            label={t('badges.attention', { count: attentionCount })}
            icon={<WarningCircleIcon aria-hidden="true" />}
          >
            {t('badges.attention', { count: attentionCount })}
          </Chip>
        </a>
      ) : null}
      {asleepSince ? (
        <Link to="/apps/$name/deploys" params={{ name: app.name }}>
          <Chip
            label={tSleep('badge.title')}
            icon={<MoonIcon aria-hidden="true" />}
          >
            <AsleepFor since={asleepSince} />
          </Chip>
        </Link>
      ) : null}
      {network?.container_port ? (
        <Link to="/apps/$name/network" params={{ name: app.name }}>
          <Chip
            label={portLabel}
            icon={<PlugsConnectedIcon aria-hidden="true" />}
          >
            {portLabel}
          </Chip>
        </Link>
      ) : null}
      {app.node_id ? (
        <Link to="/nodes/$id" params={{ id: app.node_id }}>
          <Chip
            label={app.node_name ?? app.node_id}
            icon={<HardDrivesIcon aria-hidden="true" />}
          >
            {app.node_name ?? app.node_id}
          </Chip>
        </Link>
      ) : null}
      {lastOk?.finished_at ? (
        <Link to="/apps/$name/deploys" params={{ name: app.name }}>
          <Chip
            label={deployedLabel}
            icon={<ClockCounterClockwiseIcon aria-hidden="true" />}
          >
            {deployedLabel} <RelativeTime at={lastOk.finished_at} />
          </Chip>
        </Link>
      ) : null}
    </div>
  )
}
