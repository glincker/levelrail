import { createFileRoute, Link } from '@tanstack/react-router'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { ModelDetailPage } from '../../components/models/ModelDetailPage'
import { parseModelTab, type ModelTab } from '../../lib/modelPresentation'
import { routeErrorMessage } from '../../lib/apiError'
import { modelDetailQueryOptions } from '../../queries/models'

export const Route = createFileRoute('/models/$name')({
  validateSearch: (search: Record<string, unknown>): { tab: ModelTab } => ({
    tab: parseModelTab(search.tab),
  }),
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(modelDetailQueryOptions(name)),
  component: ModelDetailRoute,
  pendingComponent: ModelDetailSkeleton,
  errorComponent: ModelDetailError,
})

function ModelDetailRoute() {
  const { name } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ModelDetailPage
      name={name}
      tab={tab}
      onTabChange={(next) => {
        void navigate({ search: { tab: next }, replace: true })
      }}
    />
  )
}

// Mirrors ModelDetailPage's back link, header, tabs list, and the
// overview tab's own stat grid plus two cards below it (the default tab).
function ModelDetailSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <Skeleton className="h-4 w-24" />
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-5 w-16 rounded-full" />
          <Skeleton className="h-5 w-20 rounded-full" />
        </div>
        <Skeleton className="h-8 w-8 rounded-md" />
      </div>
      <div className="flex gap-4 border-b border-border pb-2">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-4 w-16" />
        ))}
      </div>
      <div className="grid gap-3 pt-2 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className="space-y-1">
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-4 w-24" />
          </div>
        ))}
      </div>
      <Skeleton className="h-32 w-full rounded-lg" />
      <Skeleton className="h-32 w-full rounded-lg" />
    </div>
  )
}

function ModelDetailError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
        <Link to="/models" className="mt-2 inline-block underline">
          Back to AI models
        </Link>
      </AlertDescription>
    </Alert>
  )
}
