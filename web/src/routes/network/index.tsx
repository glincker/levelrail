import { createFileRoute, Link } from '@tanstack/react-router'
import { ShareNetworkIcon } from '@phosphor-icons/react/dist/ssr'
import {
  networkTopologyQueryOptions,
  useNetworkTopology,
} from '../../queries/networkTopology'
import { NetworkTopologyView } from '../../components/network/NetworkTopologyView'
import { routeErrorMessage } from '../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

// The whole-mesh topology page: one "VPC" (the flat mesh every node
// joins) with each node rendered as a zone. See
// web/src/lib/networkTopologyLayout.ts for the grouping this route hands
// off to NetworkTopologyView, and internal/api/network_topology.go for
// the read-only API it's built from.
export const Route = createFileRoute('/network/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(networkTopologyQueryOptions()),
  component: NetworkPage,
  pendingComponent: NetworkPageSkeleton,
  errorComponent: NetworkPageError,
})

function NetworkPage() {
  const { data: topology } = useNetworkTopology()
  const empty =
    topology.nodes.length === 0 &&
    topology.apps.length === 0 &&
    topology.databases.length === 0

  return (
    <div className="space-y-4">
      <PageHeader
        title="Network"
        description={`Everything on the mesh, grouped by node. Zone ${topology.zone}.`}
      />

      {empty ? (
        <EmptyState
          icon={<ShareNetworkIcon className="size-5" />}
          title="Nothing on the mesh yet"
          description="Add a node and deploy an app or database to see how the fleet is connected."
          action={
            <Link
              to="/nodes"
              className="text-sm font-medium text-foreground underline underline-offset-4"
            >
              Go to nodes
            </Link>
          }
        />
      ) : (
        <NetworkTopologyView topology={topology} />
      )}
    </div>
  )
}

// The topology diagram's connector lines aren't worth faking, so this
// mirrors just the outer chrome: header, then a row of zone-shaped boxes.
function NetworkPageSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div>
        <Skeleton className="h-6 w-24" />
        <Skeleton className="mt-1 h-4 w-72" />
      </div>
      <div className="flex flex-wrap gap-4">
        {Array.from({ length: 3 }, (_, i) => (
          <div
            key={i}
            className="min-w-[18rem] flex-1 space-y-3 rounded-xl border border-dashed border-border bg-muted/20 p-3"
          >
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-16 w-full rounded-lg" />
            <Skeleton className="h-16 w-full rounded-lg" />
          </div>
        ))}
      </div>
    </div>
  )
}

function NetworkPageError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
      </AlertDescription>
    </Alert>
  )
}
