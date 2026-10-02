import {
  ClockCounterClockwiseIcon,
  WifiHighIcon,
  WifiSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useMeshStatus } from '../../queries/mesh'
import type { MeshPeerResource } from '../../types/mesh'
import { DeleteNodeDialog } from '../DeleteNodeDialog'
import { RejoinMeshButton } from '../RejoinMeshButton'
import { RotateMeshKeyDialog } from '../RotateMeshKeyDialog'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// One row's worth of columns, actions in their own trailing column (not
// folded into the health cell like NodeMeshCard's own PeerRow): this
// panel is the fleet-wide action surface the per-node mesh card doesn't
// need to be, since a node detail page already has the node's own
// cordon/drain/delete controls elsewhere on that page.
const MESH_PEER_GRID =
  'grid grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)_minmax(0,1.2fr)_auto] items-center gap-3'

function formatHandshake(iso?: string): string {
  if (!iso) {
    return 'Never'
  }
  return new Date(iso).toLocaleString()
}

function PeerActionsRow({
  peer,
  isLocal,
}: {
  peer: MeshPeerResource
  isLocal: boolean
}) {
  const name = peer.name || peer.node_id
  return (
    <div className="flex flex-wrap items-center justify-end gap-1.5">
      <RejoinMeshButton nodeId={peer.node_id} nodeName={name} />
      <RotateMeshKeyDialog nodeId={peer.node_id} nodeName={name} />
      {isLocal ? null : <DeleteNodeDialog id={peer.node_id} name={name} />}
    </div>
  )
}

function PeerRow({
  peer,
  isLocal,
}: {
  peer: MeshPeerResource
  isLocal: boolean
}) {
  const label = peer.name || peer.node_id
  return (
    <div
      className={`${MESH_PEER_GRID} border-b border-border px-3 py-2 text-sm last:border-b-0`}
    >
      <div className="min-w-0">
        <div className="truncate font-medium text-foreground">
          {label}
          {isLocal ? (
            <span className="ml-1.5 text-xs text-muted-foreground">
              (this control plane)
            </span>
          ) : null}
        </div>
        <div className="truncate font-mono text-xs text-muted-foreground">
          {peer.mesh_address || 'No mesh address yet'}
        </div>
      </div>
      <div>
        {!peer.live ? (
          <Badge variant="muted">
            <ClockCounterClockwiseIcon className="size-3" aria-hidden="true" />
            Not peered yet
          </Badge>
        ) : peer.healthy ? (
          <Badge variant="success">
            <WifiHighIcon className="size-3" aria-hidden="true" />
            Healthy
          </Badge>
        ) : (
          <Badge variant="destructive">
            <WifiSlashIcon className="size-3" aria-hidden="true" />
            Stale
          </Badge>
        )}
      </div>
      <div className="truncate text-xs text-muted-foreground">
        {peer.live
          ? formatHandshake(peer.last_handshake_at)
          : 'No live peer entry'}
      </div>
      <PeerActionsRow peer={peer} isLocal={isLocal} />
    </div>
  )
}

// The network topology page's own action surface: every node on the
// mesh, its live connection health, and the three operator actions this
// page was missing entirely (rotate key, force a resync, remove the
// node). GET /api/v1/mesh degrading to a 501 (mesh networking disabled)
// is handled one level up, by NetworkPage's own banner off
// topology.mesh_enabled, so this panel renders nothing for that case
// rather than duplicating the message.
export function MeshPeersPanel({ localNodeId }: { localNodeId?: string }) {
  const { data, isPending, isError } = useMeshStatus()

  if (isPending) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Mesh peers</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </CardContent>
      </Card>
    )
  }

  if (isError || !data) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Mesh peers</CardTitle>
      </CardHeader>
      <CardContent>
        {data.peers.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No peers configured yet.
          </p>
        ) : (
          <div className="overflow-hidden rounded-lg border border-border">
            <div
              className={`${MESH_PEER_GRID} border-b border-border bg-muted/40 px-3 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
            >
              <span>Peer</span>
              <span>Health</span>
              <span>Last handshake</span>
              <span className="text-right">Actions</span>
            </div>
            {data.peers.map((peer) => (
              <PeerRow
                key={peer.node_id || peer.public_key}
                peer={peer}
                isLocal={
                  peer.node_id !== '' &&
                  peer.node_id === (localNodeId ?? data.local_node_id)
                }
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
