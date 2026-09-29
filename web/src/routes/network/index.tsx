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
import { PageSpinner } from '@/components/ui/page-spinner'

// The whole-mesh topology page: one "VPC" (the flat mesh every node
// joins) with each node rendered as a zone. See
// web/src/lib/networkTopologyLayout.ts for the grouping this route hands
// off to NetworkTopologyView, and internal/api/network_topology.go for
// the read-only API it's built from.
export const Route = createFileRoute('/network/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(networkTopologyQueryOptions()),
  component: NetworkPage,
  pendingComponent: PageSpinner,
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
      <div>
        <h1 className="text-lg font-semibold text-foreground">Network</h1>
        <p className="text-sm text-muted-foreground">
          Everything on the mesh, grouped by node. Zone {topology.zone}.
        </p>
      </div>

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

function NetworkPageError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
      </AlertDescription>
    </Alert>
  )
}
