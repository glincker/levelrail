import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { appListQueryOptions } from '../queries/apps'
import { DashboardOverview } from '../components/DashboardOverview'

// Same loader/useSuspenseQuery split as routes/apps/index.tsx: the
// loader primes the cache, the component only reads it.
export const Route = createFileRoute('/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(appListQueryOptions()),
  component: DashboardPage,
  pendingComponent: DashboardPending,
})

function DashboardPage() {
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  return <DashboardOverview apps={apps} />
}

function DashboardPending() {
  return (
    <div className="space-y-6">
      <div className="h-6 w-32 animate-pulse rounded bg-muted" />
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <div key={i} className="h-16 animate-pulse rounded-xl bg-muted" />
        ))}
      </div>
      <div className="h-40 animate-pulse rounded-lg bg-muted" />
    </div>
  )
}
