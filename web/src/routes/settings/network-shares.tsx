import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { networkShareListQueryOptions } from '../../queries/networkShares'
import { NetworkShareTable } from '../../components/NetworkShareTable'
import { CreateNetworkShareDialog } from '../../components/CreateNetworkShareDialog'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

// Account-level, same reasoning settings/registry-credentials.tsx's own
// doc comment gives: a network share isn't scoped to one app, it's a
// volume source any app's service can attach to.
export const Route = createFileRoute('/settings/network-shares')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(networkShareListQueryOptions()),
  component: NetworkSharesPage,
  pendingComponent: NetworkSharesPending,
})

function NetworkSharesPage() {
  const { data: shares } = useSuspenseQuery(networkShareListQueryOptions())

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <HardDrivesIcon className="size-4" />
        </div>
        <div className="min-w-0 flex-1">
          <PageHeader
            title="Network shares"
            description="NFS and CIFS/SMB exports, mounted as Docker volumes using Docker's own local-driver network passthrough. Attach one as a volume source when configuring an app's volumes."
            actions={<CreateNetworkShareDialog />}
          />
        </div>
      </div>
      <NetworkShareTable
        shares={shares}
        action={<CreateNetworkShareDialog />}
      />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching
// NetworkShareTable's own 5-column shape so the skeleton doesn't jump
// when real rows swap in.
function NetworkSharesPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">Network shares</h1>
      <TableSkeleton columnCount={5} />
    </div>
  )
}
