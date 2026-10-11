import { useMemo } from 'react'
import { useDatabases } from './databases'
import { useAppResourceUsage } from './appResourceUsage'
import { useFleetResourceUsage } from './fleetResourceUsage'
import { useAppsMetrics } from './appsMetrics'
import { useUsageSummary } from './usageSummary'
import {
  buildConsumers,
  buildHostGauges,
  buildMemoryBreakdown,
  type Consumer,
  type HostGauge,
  type MemoryBreakdown,
} from '../lib/fleetResources'

export interface DashboardResources {
  isPending: boolean
  gauges: HostGauge[]
  consumers: Consumer[]
  breakdown: MemoryBreakdown | undefined
  databaseCount: number
}

// One hook over the existing shared queries (same keys as the node and app
// pages, so nothing refetches twice). Each source degrades alone: a control
// plane without telemetry still gets the database rows from usage/summary.
export function useDashboardResources(): DashboardResources {
  const fleet = useFleetResourceUsage()
  const apps = useAppResourceUsage()
  const usage = useUsageSummary()
  const metrics = useAppsMetrics()
  const dbs = useDatabases()

  return useMemo(() => {
    const consumers = buildConsumers(
      apps.data ?? [],
      usage.data?.databases ?? [],
      metrics.data,
    )
    return {
      isPending: fleet.isPending && apps.isPending && usage.isPending,
      gauges: buildHostGauges(
        fleet.data?.nodes ?? [],
        usage.data?.local_cpu_cores,
      ),
      consumers,
      breakdown: buildMemoryBreakdown(fleet.data, consumers),
      databaseCount: dbs.data?.length ?? 0,
    }
  }, [
    fleet.data,
    fleet.isPending,
    apps.data,
    apps.isPending,
    usage.data,
    usage.isPending,
    metrics.data,
    dbs.data,
  ])
}
