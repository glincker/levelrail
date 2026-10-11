import { createFileRoute } from '@tanstack/react-router'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../queries/deployAttempts'
import { MetricsDashboard } from '../../../components/MetricsDashboard'
import { parseMetricsSearch } from '../../../lib/observabilitySearch'
import { Skeleton } from '@/components/ui/skeleton'

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
  validateSearch: parseMetricsSearch,
  component: MetricsSection,
  pendingComponent: MetricsSectionSkeleton,
})

function MetricsSection() {
  const { name } = Route.useParams()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const { data: deployAttempts } = useDeployAttempts(name)

  return (
    <MetricsDashboard
      appName={name}
      deployAttempts={deployAttempts}
      search={search}
      onSearchChange={(next) => {
        void navigate({ search: next, replace: true })
      }}
    />
  )
}

function ChartTileSkeleton() {
  return (
    <div className="space-y-2 rounded-lg border border-border p-3">
      <Skeleton className="h-3 w-24" />
      <Skeleton className="h-40 w-full" />
    </div>
  )
}

// Mirrors MetricsDashboard's own two chart grids (CHART_GROUPS, then
// RequestMetricsSection's own three tiles), reusing one tile skeleton
// since both grids render the exact same MetricChartCard shape.
function MetricsSectionSkeleton() {
  return (
    <div className="rounded-lg border border-border p-4" aria-hidden="true">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-2">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-3 w-64" />
        </div>
        <Skeleton className="h-8 w-40 rounded-md" />
      </div>
      <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        {Array.from({ length: 5 }, (_, i) => (
          <ChartTileSkeleton key={i} />
        ))}
      </div>
      <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        {Array.from({ length: 3 }, (_, i) => (
          <ChartTileSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}
