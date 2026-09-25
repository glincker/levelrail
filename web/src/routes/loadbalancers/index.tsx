import { createFileRoute } from '@tanstack/react-router'
import { LoadBalancerOverview } from '../../components/LoadBalancerOverview'
import { PageSpinner } from '@/components/ui/page-spinner'

export const Route = createFileRoute('/loadbalancers/')({
  component: LoadBalancerOverview,
  pendingComponent: PageSpinner,
})
