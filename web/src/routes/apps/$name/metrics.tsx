import { createFileRoute } from '@tanstack/react-router'
import { useDeployStatus } from '../../../queries/deploys'
import { MetricsDashboard } from '../../../components/MetricsDashboard'

// Former "metrics" tab, now a real deep-linkable route. Reads conditions
// from the query cache the parent layout route's loader already primed;
// MetricsDashboard only needs appName plus conditions, not the full
// AppDetail, so this route skips useApp entirely.
export const Route = createFileRoute('/apps/$name/metrics')({
  component: MetricsSection,
})

function MetricsSection() {
  const { name } = Route.useParams()
  const { data: conditions } = useDeployStatus(name)

  return <MetricsDashboard appName={name} conditions={conditions} />
}
