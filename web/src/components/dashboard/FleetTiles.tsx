import { useMemo } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  CpuIcon,
  HardDrivesIcon,
  PackageIcon,
  PulseIcon,
  RocketLaunchIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { MetricTile } from '@/components/kit'
import type { AppListEntry } from '../../types/appDetail'
import type { AppsSummary } from '../../queries/appPages'
import { useTranslation } from 'react-i18next'
import { ERROR_RATE_MIN_REQUESTS, errorTone } from '../../lib/fleetThresholds'
import {
  deploysPerHour,
  errorReading,
  flattenAttempts,
  nodeCounts,
  pickSampleApps,
  summarizeTraffic,
} from '../../lib/fleetStats'
import { useMinuteNow } from '../../hooks/useMinuteNow'
import { nodeListQueryOptions } from '../../queries/nodes'
import { useGpuNodes } from '../../queries/models'
import { useFleetDeploys } from '../../queries/fleetTraffic'
import { useAppsMetrics } from '../../queries/appsMetrics'
import { InstanceWideLabel } from './ScopeIndicator'

export const SAMPLE_APP_CAP = 8

export function FleetTiles({
  summary,
  sample,
}: {
  summary: AppsSummary
  sample: AppListEntry[]
}) {
  const navigate = useNavigate()
  const { t: tr } = useTranslation('dashboard')
  const now = useMinuteNow()
  const names = useMemo(() => pickSampleApps(sample, SAMPLE_APP_CAP), [sample])
  const traffic = useAppsMetrics()
  const deploys = useFleetDeploys(names)
  const nodes = useQuery({
    ...nodeListQueryOptions(),
    retry: false,
    refetchInterval: 30_000,
  })
  const gpus = useGpuNodes()

  const buckets = summary
  const t = summarizeTraffic(traffic.data ?? [])
  const d = deploysPerHour(flattenAttempts(deploys.byApp), now)
  const nodeTotals = nodeCounts(nodes.data ?? [])
  const errors = errorReading(t, ERROR_RATE_MIN_REQUESTS)
  const gpuCount = (gpus.data ?? []).reduce(
    (sum, g) => sum + (g.present ? g.gpu_count : 0),
    0,
  )
  const sampled = tr('tiles.lastHour')

  return (
    <div className="@container">
      <div className="grid grid-cols-2 gap-3 @2xl:grid-cols-3 @5xl:grid-cols-5">
        <MetricTile
          label={tr('tiles.appsRunning')}
          icon={<PackageIcon className="size-3.5" />}
          value={`${buckets.running} / ${summary.total}`}
          tone={buckets.failing > 0 ? 'danger' : 'success'}
          onClick={() => void navigate({ to: '/apps' })}
        />
        <MetricTile
          label={tr('tiles.requests')}
          icon={<PulseIcon className="size-3.5" />}
          value={
            t.ratePerSec > 0
              ? Number(t.ratePerSec.toFixed(1))
              : tr('tiles.idle')
          }
          unit={t.ratePerSec > 0 ? 'req/s' : undefined}
          series={t.ratePerSec > 0 ? t.series : undefined}
          cta={
            t.ratePerSec > 0
              ? undefined
              : {
                  label: tr('tiles.addDomain'),
                  onAction: () => void navigate({ to: '/domains' }),
                }
          }
          tone="accent"
          info={<span className="sr-only">{sampled}</span>}
          loading={traffic.isPending}
        />
        <MetricTile
          label={tr('tiles.errorRate')}
          icon={<WarningIcon className="size-3.5" />}
          value={
            errors.kind === 'value'
              ? Number(errors.pct.toFixed(2))
              : tr('tiles.noTraffic')
          }
          unit={errors.kind === 'value' ? '%' : undefined}
          tone={errors.kind === 'value' ? errorTone(errors.pct) : 'neutral'}
          loading={traffic.isPending}
        />
        <MetricTile
          label={tr('tiles.deploys24h')}
          icon={<RocketLaunchIcon className="size-3.5" />}
          value={d.total}
          series={d.total > 0 ? d.buckets : undefined}
          tone="info"
          cta={
            d.total > 0
              ? undefined
              : {
                  label: tr('tiles.deployApp'),
                  onAction: () => void navigate({ to: '/apps' }),
                }
          }
          loading={deploys.isPending}
        />
        <MetricTile
          label={tr('tiles.nodesOnline')}
          icon={<HardDrivesIcon className="size-3.5" />}
          value={`${nodeTotals.online} / ${nodeTotals.total}`}
          tone={nodeTotals.online < nodeTotals.total ? 'warning' : 'success'}
          loading={nodes.isPending}
          note={<InstanceWideLabel />}
          onClick={() => void navigate({ to: '/nodes' })}
        />
        {gpuCount > 0 ? (
          <MetricTile
            label="GPUs"
            icon={<CpuIcon className="size-3.5" />}
            value={gpuCount}
            tone="accent"
          />
        ) : null}
      </div>
    </div>
  )
}
