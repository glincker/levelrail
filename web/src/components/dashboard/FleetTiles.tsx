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
import { countBuckets } from '../../lib/appsListView'
import { errorTone } from '../../lib/fleetThresholds'
import {
  deploysPerHour,
  flattenAttempts,
  pickSampleApps,
  summarizeTraffic,
} from '../../lib/fleetStats'
import { useMinuteNow } from '../../hooks/useMinuteNow'
import { nodeListQueryOptions } from '../../queries/nodes'
import { useGpuNodes } from '../../queries/models'
import { useFleetDeploys, useFleetTraffic } from '../../queries/fleetTraffic'

export const SAMPLE_APP_CAP = 8

export function FleetTiles({ apps }: { apps: AppListEntry[] }) {
  const navigate = useNavigate()
  const now = useMinuteNow()
  const names = useMemo(() => pickSampleApps(apps, SAMPLE_APP_CAP), [apps])
  const traffic = useFleetTraffic(names)
  const deploys = useFleetDeploys(names)
  const nodes = useQuery({
    ...nodeListQueryOptions(),
    retry: false,
    refetchInterval: 30_000,
  })
  const gpus = useGpuNodes()

  const buckets = countBuckets(apps)
  const t = summarizeTraffic(traffic.series)
  const d = deploysPerHour(flattenAttempts(deploys.byApp), now)
  const nodeList = nodes.data ?? []
  const online = nodeList.filter((n) => n.status === 'online').length
  const gpuCount = (gpus.data ?? []).reduce(
    (sum, g) => sum + (g.present ? g.gpu_count : 0),
    0,
  )
  const sampled =
    apps.length > names.length
      ? `Last hour, sampled from ${names.length} of ${apps.length} apps.`
      : 'Last hour, all apps.'

  return (
    <div className="grid grid-cols-[repeat(auto-fit,minmax(10rem,1fr))] gap-3">
      <MetricTile
        label="Apps running"
        icon={<PackageIcon className="size-3.5" />}
        value={`${buckets.running} / ${apps.length}`}
        tone={buckets.failing > 0 ? 'danger' : 'success'}
        onClick={() => void navigate({ to: '/apps' })}
      />
      <MetricTile
        label="Requests"
        icon={<PulseIcon className="size-3.5" />}
        value={Number(t.ratePerSec.toFixed(1))}
        unit="req/s"
        series={t.series}
        tone="accent"
        info={<span className="sr-only">{sampled}</span>}
        loading={traffic.isPending}
      />
      <MetricTile
        label="Error rate"
        icon={<WarningIcon className="size-3.5" />}
        value={Number(t.errorPct.toFixed(2))}
        unit="%"
        tone={errorTone(t.errorPct)}
        loading={traffic.isPending}
      />
      <MetricTile
        label="Deploys 24h"
        icon={<RocketLaunchIcon className="size-3.5" />}
        value={d.total}
        series={d.buckets}
        tone="info"
        loading={deploys.isPending}
      />
      {nodes.data ? (
        <MetricTile
          label="Nodes online"
          icon={<HardDrivesIcon className="size-3.5" />}
          value={`${online} / ${nodeList.length}`}
          tone={online < nodeList.length ? 'warning' : 'success'}
          onClick={() => void navigate({ to: '/nodes' })}
        />
      ) : null}
      {gpuCount > 0 ? (
        <MetricTile
          label="GPUs"
          icon={<CpuIcon className="size-3.5" />}
          value={gpuCount}
          tone="accent"
        />
      ) : null}
    </div>
  )
}
