import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { networkShareListQueryOptions } from '../../queries/networkShares'
import { NetworkSharesPage } from '../../components/networkShares/NetworkSharesPage'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

// Account-level, same reasoning settings/registry-credentials.tsx's own
// doc comment gives: a network share isn't scoped to one app, it's a
// volume source any app's service can attach to.
export const Route = createFileRoute('/settings/network-shares')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(networkShareListQueryOptions()),
  component: NetworkSharesRoute,
  pendingComponent: NetworkSharesPending,
})

function NetworkSharesRoute() {
  const { data: shares } = useSuspenseQuery(networkShareListQueryOptions())
  return <NetworkSharesPage shares={shares} />
}

// Route-level fallback for the loader's pending phase, matching
// NetworkShareTable's own 5-column shape so the skeleton doesn't jump
// when real rows swap in.
function NetworkSharesPending() {
  return (
    <div className="space-y-6">
      <PageHeader title="Network shares" />
      <TableSkeleton columnCount={5} />
    </div>
  )
}
