import { createFileRoute } from '@tanstack/react-router'
import { MonitoringTabs } from '../../../components/databases/home/MonitoringTabs'
import { DatabaseMonitoringPage } from '../../../components/databases/monitoring/DatabaseMonitoringPage'

// Database-scoped metrics section, the database-kind counterpart to
// routes/apps/$name/metrics.tsx. No loader needed: unlike the app route
// (which primes deploy-attempts history for the chart's deploy-marker
// overlay), DatabaseMetricsDashboard has no equivalent history to prime,
// databases have no deploys.
export const Route = createFileRoute('/databases/$name/metrics')({
  component: MetricsSection,
})

function MetricsSection() {
  const { name } = Route.useParams()
  return (
    <>
      <MonitoringTabs name={name} />
      <DatabaseMonitoringPage databaseName={name} />
    </>
  )
}
