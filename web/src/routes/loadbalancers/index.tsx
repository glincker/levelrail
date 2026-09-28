import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import { LoadBalancerOverview } from '../../components/LoadBalancerOverview'
import { PageSpinner } from '@/components/ui/page-spinner'

export const Route = createFileRoute('/loadbalancers/')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'load-balancer'),
  component: LoadBalancerOverview,
  pendingComponent: PageSpinner,
})
