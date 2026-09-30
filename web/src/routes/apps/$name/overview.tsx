import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { deployAttemptsQueryOptions } from '../../../queries/deployAttempts'
import { useDeployProgress } from '../../../hooks/useDeployProgress'
import { OverviewPage } from '../../../components/overview/OverviewPage'
import { Skeleton } from '@/components/ui/skeleton'

// The loader primes deploy attempts (same query the Deploys tab uses); app
// and conditions come from the cache the parent layout route already primed.
export const Route = createFileRoute('/apps/$name/overview')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
  component: OverviewSection,
  pendingComponent: AppOverviewSkeleton,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { attempts, conditions } = useDeployProgress(name)
  return <OverviewPage app={app} conditions={conditions} attempts={attempts} />
}

// Mirrors OverviewPage's own section order (hero, setup ring, suggestions,
// tile grid, timeline/logs columns, details) so the layout does not jump
// once real data swaps in.
function AppOverviewSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <div className="space-y-4 rounded-2xl border border-border bg-card p-4 sm:p-6">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-5 w-20 rounded-full" />
        </div>
        <Skeleton className="h-4 w-64" />
        <div className="flex gap-2">
          <Skeleton className="h-9 w-24 rounded-md" />
          <Skeleton className="h-9 w-24 rounded-md" />
        </div>
      </div>

      <Skeleton className="h-2 w-full rounded-full" />

      <div className="space-y-2 rounded-2xl border border-border bg-card p-4">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-4 w-full" />
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
        {Array.from({ length: 5 }, (_, i) => (
          <div
            key={i}
            className="space-y-2 rounded-xl border border-border p-3"
          >
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-6 w-20" />
          </div>
        ))}
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="space-y-3 rounded-2xl border border-border bg-card p-4">
          <Skeleton className="h-4 w-32" />
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
        <div className="space-y-3 rounded-2xl border border-border bg-card p-4">
          <Skeleton className="h-4 w-24" />
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-3 w-full" />
          ))}
        </div>
      </div>

      <div className="space-y-3 rounded-2xl border border-border bg-card p-4">
        <Skeleton className="h-4 w-28" />
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
          {Array.from({ length: 6 }, (_, i) => (
            <div key={i} className="space-y-1">
              <Skeleton className="h-3 w-16" />
              <Skeleton className="h-4 w-24" />
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
