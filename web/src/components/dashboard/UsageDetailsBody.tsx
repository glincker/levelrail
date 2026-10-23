import { Suspense, lazy } from 'react'
import { useQuery } from '@tanstack/react-query'
import { appListQueryOptions } from '../../queries/apps'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { FleetUtilizationSummary } from '../FleetUtilizationSummary'
import { TopResourceConsumers } from '../TopResourceConsumers'
import { ReservedUsageCard } from './ReservedUsageCard'
import { StorageUsageCard } from './StorageUsageCard'
import { SleepFleetCard } from './SleepFleetCard'
import { InstanceWideLabel } from './ScopeIndicator'

const FleetResourceChart = lazy(() =>
  import('../FleetResourceChart').then((m) => ({
    default: m.FleetResourceChart,
  })),
)

function UsageApps() {
  useEnvironmentScope()
  const { data: apps = [] } = useQuery(appListQueryOptions())
  return (
    <>
      <TopResourceConsumers apps={apps} />
      <ReservedUsageCard apps={apps} />
    </>
  )
}

/** Body of the collapsed "Usage details" section, split out so it loads on first expand. */
export function UsageDetailsBody() {
  return (
    <div className="space-y-4">
      <InstanceWideLabel />
      <FleetUtilizationSummary />
      <Suspense fallback={null}>
        <FleetResourceChart />
      </Suspense>
      <UsageApps />
      <StorageUsageCard />
      <SleepFleetCard />
    </div>
  )
}
