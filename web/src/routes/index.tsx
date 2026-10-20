import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  appSampleQueryOptions,
  appsSummaryQueryOptions,
} from '../queries/appPages'
import { onboardingQueryOptions } from '../queries/onboarding'
import { userListQueryOptions } from '../queries/users'
import { DashboardHome } from '../components/dashboard/DashboardHome'
import { PanelHost } from '../components/plugins/PanelHost'
import { SkeletonLine, SkeletonTile } from '@/components/kit'

// Same loader/useSuspenseQuery split as routes/apps/index.tsx: the
// loader primes the cache, the component only reads it. onboarding is
// primed alongside the apps summary (not fetched optionally/client-side) because it
// gates which component DashboardOverview renders on a zero-app
// instance, not merely decorative.
export const Route = createFileRoute('/')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(appsSummaryQueryOptions()),
      queryClient.ensureQueryData(appSampleQueryOptions()),
      queryClient.ensureQueryData(onboardingQueryOptions()),
      queryClient.prefetchQuery(userListQueryOptions()),
    ]),
  component: DashboardPage,
  pendingComponent: DashboardPending,
})

function DashboardPage() {
  const { data: summary } = useSuspenseQuery({
    ...appsSummaryQueryOptions(),
    refetchInterval: 15_000,
  })
  const { data: sample } = useSuspenseQuery({
    ...appSampleQueryOptions(),
    refetchInterval: 15_000,
  })
  const { data: onboarding } = useSuspenseQuery(onboardingQueryOptions())
  return (
    <div className="space-y-8">
      <DashboardHome
        summary={summary}
        sample={sample}
        onboarding={onboarding}
      />
      <PanelHost slot="dashboard" />
    </div>
  )
}

function DashboardPending() {
  return (
    <div className="space-y-8" aria-busy="true">
      <SkeletonLine width={260} className="h-6" />
      <div className="grid grid-cols-[repeat(auto-fit,minmax(10rem,1fr))] gap-3">
        {Array.from({ length: 5 }, (_, i) => (
          <SkeletonTile key={i} />
        ))}
      </div>
      <SkeletonTile className="h-40" />
    </div>
  )
}
