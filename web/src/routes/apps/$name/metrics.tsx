import { createFileRoute } from '@tanstack/react-router'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../queries/deployAttempts'
import { MetricsDashboard } from '../../../components/MetricsDashboard'

// Former "metrics" tab, now a real deep-linkable route. MetricsDashboard
// now plots real multi-attempt deploy history (see that component's own
// header comment), so this route's own loader primes
// deployAttemptsQueryOptions, the same query deploys/index.tsx already
// primes for the Deploys section; neither route shares the other's
// cache entry with the parent layout route the way conditions used to,
// since deploy attempts are this route's own concern, not something the
// parent `/apps/$name` layout needs for its header.
export const Route = createFileRoute('/apps/$name/metrics')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
  component: MetricsSection,
})

function MetricsSection() {
  const { name } = Route.useParams()
  const { data: deployAttempts } = useDeployAttempts(name)

  return <MetricsDashboard appName={name} deployAttempts={deployAttempts} />
}
