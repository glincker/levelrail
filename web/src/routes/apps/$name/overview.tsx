import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { deployAttemptsQueryOptions } from '../../../queries/deployAttempts'
import { useDeployProgress } from '../../../hooks/useDeployProgress'
import { OverviewPage } from '../../../components/overview/OverviewPage'
import { PageSpinner } from '@/components/ui/page-spinner'

// The loader primes deploy attempts (same query the Deploys tab uses); app
// and conditions come from the cache the parent layout route already primed.
export const Route = createFileRoute('/apps/$name/overview')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
  component: OverviewSection,
  pendingComponent: PageSpinner,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { attempts, conditions } = useDeployProgress(name)
  return <OverviewPage app={app} conditions={conditions} attempts={attempts} />
}
