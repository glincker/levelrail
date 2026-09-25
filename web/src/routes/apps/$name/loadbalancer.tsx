import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { AppLoadBalancerCard } from '../../../components/AppLoadBalancerCard'
import { PageSpinner } from '@/components/ui/page-spinner'

// The load balancer tab: algorithm, weights, health checks and the live
// upstream table for this app's replicas. App data is already primed by
// the parent layout route's loader.
export const Route = createFileRoute('/apps/$name/loadbalancer')({
  component: LoadBalancerSection,
  pendingComponent: PageSpinner,
})

function LoadBalancerSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return <AppLoadBalancerCard appName={name} replicas={app.replicas} />
}
