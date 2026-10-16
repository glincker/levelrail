import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ClockCounterClockwiseIcon,
  HardDrivesIcon,
  PlugsConnectedIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { RelativeTime } from '../kit/RelativeTime'
import { useAppNetwork } from '../../queries/appNetwork'
import { useDeployAttempts } from '../../queries/deployAttempts'
import type { AppDetail } from '../../types/appDetail'

/** Compact facts for the top right of every app page; each links to its detail. */
export function AppHeaderBadges({
  app,
  attentionCount = 0,
}: {
  app: Pick<AppDetail, 'name' | 'node_id'>
  attentionCount?: number
}) {
  const { t } = useTranslation('appAccess')
  const { data: network } = useAppNetwork(app.name)
  const { data: attempts } = useDeployAttempts(app.name)
  const lastOk = attempts?.find((a) => a.status === 'succeeded')
  return (
    <div className="flex flex-wrap items-center gap-1.5 lg:justify-end">
      {attentionCount > 0 ? (
        <a href="#overview-attention">
          <Badge variant="destructive">
            <WarningCircleIcon aria-hidden="true" />
            {t('badges.attention', { count: attentionCount })}
          </Badge>
        </a>
      ) : null}
      {network?.container_port ? (
        <Link to="/apps/$name/network" params={{ name: app.name }}>
          <Badge variant="muted">
            <PlugsConnectedIcon aria-hidden="true" />
            {t('badges.port', { port: network.container_port })}
          </Badge>
        </Link>
      ) : null}
      {app.node_id ? (
        <Badge variant="muted">
          <HardDrivesIcon aria-hidden="true" />
          {app.node_id}
        </Badge>
      ) : null}
      {lastOk?.finished_at ? (
        <Link to="/apps/$name/deploys" params={{ name: app.name }}>
          <Badge variant="muted">
            <ClockCounterClockwiseIcon aria-hidden="true" />
            {t('badges.deployed')} <RelativeTime at={lastOk.finished_at} />
          </Badge>
        </Link>
      ) : null}
    </div>
  )
}
