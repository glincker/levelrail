import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  appSampleQueryOptions,
  appsSummaryQueryOptions,
} from '../queries/appPages'
import { userListQueryOptions } from '../queries/users'
import { useEnvironmentScope } from '../lib/environmentScope'
import { DashboardHome } from '../components/dashboard/DashboardHome'
import { PanelHost } from '../components/plugins/PanelHost'
import { SkeletonLine, SkeletonTile } from '@/components/kit'

// Same loader/useSuspenseQuery split as routes/apps/index.tsx: the
// loader primes the cache, the component only reads it.
interface HomeSearch {
  board?: string
}

export const Route = createFileRoute('/')({
  validateSearch: (search: Record<string, unknown>): HomeSearch =>
    typeof search.board === 'string' && search.board
      ? { board: search.board }
      : {},
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(appsSummaryQueryOptions()),
      queryClient.ensureQueryData(appSampleQueryOptions()),
      queryClient.prefetchQuery(userListQueryOptions()),
    ]),
  component: DashboardPage,
  pendingComponent: DashboardPending,
})

function DashboardPage() {
  useEnvironmentScope()
  const { data: summary } = useSuspenseQuery({
    ...appsSummaryQueryOptions(),
    refetchInterval: 15_000,
  })
  const { data: sample } = useSuspenseQuery({
    ...appSampleQueryOptions(),
    refetchInterval: 15_000,
  })
  const { board } = Route.useSearch()
  return (
    <div className="space-y-8">
      <DashboardHome summary={summary} sample={sample} board={board} />
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
