import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import { LoadBalancerOverview } from '../../components/LoadBalancerOverview'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { Skeleton } from '@/components/ui/skeleton'

export const Route = createFileRoute('/loadbalancers/')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'load-balancer'),
  component: LoadBalancerOverview,
  pendingComponent: LoadBalancerOverviewSkeleton,
})

// Mirrors LoadBalancerOverview's own header/search row above its virtualized
// table, using the shared TableSkeleton for the row placeholders.
function LoadBalancerOverviewSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-2">
          <Skeleton className="h-6 w-40" />
          <Skeleton className="h-4 w-80" />
        </div>
        <Skeleton className="h-8 w-40 rounded-md" />
      </div>
      <Skeleton className="h-9 w-64 rounded-md" />
      <TableSkeleton columnCount={6} rowCount={6} />
    </div>
  )
}
