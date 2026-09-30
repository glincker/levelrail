import { requireExperimental } from '../../../lib/experimental'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { LoadBalancerPage } from '../../../components/loadbalancer/LoadBalancerPage'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { Skeleton } from '@/components/ui/skeleton'

export const Route = createFileRoute('/apps/$name/loadbalancer')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'load-balancer'),
  component: LoadBalancerSection,
  pendingComponent: LoadBalancerSectionSkeleton,
})

function LoadBalancerSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const navigate = useNavigate()

  return (
    <LoadBalancerPage
      appName={name}
      replicas={app.replicas}
      onSetReplicas={() =>
        void navigate({ to: '/apps/$name/deploy-settings', params: { name } })
      }
    />
  )
}

// Mirrors LoadBalancerPage's configured state: header row, a topology
// tile beside a metrics tile, then the upstream table.
function LoadBalancerSectionSkeleton() {
  return (
    <div className="space-y-5" aria-hidden="true">
      <div className="flex items-center justify-between gap-3">
        <Skeleton className="h-6 w-40" />
        <div className="flex items-center gap-2">
          <Skeleton className="h-8 w-24 rounded-md" />
          <Skeleton className="h-8 w-24 rounded-md" />
        </div>
      </div>
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_220px]">
        <Skeleton className="h-48 w-full rounded-2xl" />
        <Skeleton className="h-48 w-full rounded-2xl" />
      </div>
      <TableSkeleton columnCount={5} rowCount={4} />
    </div>
  )
}
