import { createFileRoute, Link } from '@tanstack/react-router'
import { ShareNetworkIcon, WifiSlashIcon } from '@phosphor-icons/react/dist/ssr'
import {
  networkTopologyQueryOptions,
  useNetworkTopology,
} from '../../queries/networkTopology'
import { MeshPeersPanel } from '../../components/network/MeshPeersPanel'
import { NetworkTopologyView } from '../../components/network/NetworkTopologyView'
import { routeErrorMessage } from '../../lib/apiError'
import type { NetworkTopologyResponse } from '../../types/networkTopology'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
  const localNodeId = topology.nodes.find((n) => n.is_local)?.id

  return (
    <div className="space-y-4">
      <PageHeader
        title="Network"
        description={`Everything on the mesh, grouped by node. Zone ${topology.zone}.`}
      />

      <MeshDisabledNotice topology={topology} />

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

      <MeshPeersPanel localNodeId={localNodeId} />
    </div>
  )
}

// Surfaces topology.mesh_enabled explicitly rather than leaving an
// operator to infer it from missing mesh addresses on the resource
// cards: a control plane with mesh networking off (APP_MESH_ENABLED
// unset, the default) still renders a perfectly normal-looking topology
// for a single node, so this distinguishes "expected, this is one node"
// from "worth knowing about, there are N registered nodes that cannot
// reach each other over the mesh."
function MeshDisabledNotice({
  topology,
}: {
  topology: NetworkTopologyResponse
}) {
  if (topology.mesh_enabled) {
    return null
  }
  const multiNode = topology.nodes.length > 1
  return (
    <Alert variant={multiNode ? 'destructive' : 'default'}>
      <WifiSlashIcon />
      <AlertTitle>Mesh networking is not enabled</AlertTitle>
      <AlertDescription>
        {multiNode
          ? `${topology.nodes.length} nodes are registered, but this control plane has no WireGuard mesh running, so apps and databases placed on different nodes may not be able to reach each other by internal DNS. Set APP_MESH_ENABLED=1 to bring it up.`
          : "Expected for a single-node deployment: everything here resolves locally, so there's no mesh cost to pay. Set APP_MESH_ENABLED=1 if this control plane will manage more than one node."}
      </AlertDescription>
    </Alert>
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
