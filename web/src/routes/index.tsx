import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { appListQueryOptions } from '../queries/apps'
import { onboardingQueryOptions } from '../queries/onboarding'
import { DashboardOverview } from '../components/DashboardOverview'

// Same loader/useSuspenseQuery split as routes/apps/index.tsx: the
// loader primes the cache, the component only reads it. onboarding is
// primed alongside apps (not fetched optionally/client-side) because it
// gates which component DashboardOverview renders on a zero-app
// instance, not merely decorative.
export const Route = createFileRoute('/')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(appListQueryOptions()),
      queryClient.ensureQueryData(onboardingQueryOptions()),
    ]),
  component: DashboardPage,
  pendingComponent: DashboardPending,
})

function DashboardPage() {
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const { data: onboarding } = useSuspenseQuery(onboardingQueryOptions())
  return (
    <DashboardOverview apps={apps} onboardingCompleted={onboarding.completed} />
  )
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
