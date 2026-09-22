import {
  CheckCircleIcon,
  ClockCounterClockwiseIcon,
  WarningCircleIcon,
  WifiHighIcon,
  WifiSlashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useMeshStatus } from '../queries/mesh'
import type { MeshPeerResource } from '../types/mesh'
import { ApiError } from '../lib/apiError'
import { RotateMeshKeyDialog } from './RotateMeshKeyDialog'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'

// Shared column grid for the peer list, the same sticky-header-plus-row
// convention NODE_LIST_GRID (NodeRow.tsx) establishes for the top-level
// node list: a fixed grid template so the header never drifts out of
// alignment with row content, rather than an actual <table>.
const PEER_LIST_GRID =
  'grid grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_minmax(0,1.4fr)_auto] items-center gap-3'

function formatHandshake(iso?: string): string {
  if (!iso) {
    return 'Never'
  }
  return new Date(iso).toLocaleString()
}

function PeerRow({ peer }: { peer: MeshPeerResource }) {
  const label = peer.name || peer.node_id
  return (
    <div
      className={`${PEER_LIST_GRID} border-b border-border px-3 py-2 text-sm last:border-b-0`}
    >
      <div className="min-w-0">
        <div className="truncate font-medium text-foreground">{label}</div>
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
      <div className="text-right font-mono text-xs text-muted-foreground">
        {peer.endpoint || ''}
      </div>
    </div>
  )
}

// Node detail route's mesh section: this control plane's own WireGuard
// mesh identity and peer list, plus a rotate-key action. See
// internal/api/mesh.go's own doc comment for the real, honest limitation
// this UI reflects rather than papers over: GET /api/v1/mesh is one
// live view (the control plane's own node), so this card renders the
// same content on every node's detail page, with the rotate action only
// meaningfully wired for the local node (see isLocal below) since
// remote-node rotation returns 501 until the agent wire extension for it
// exists.
export function NodeMeshCard({
  nodeId,
  nodeName,
  isLocal,
}: {
  nodeId: string
  nodeName: string
  isLocal: boolean
}) {
  const { data, isPending, isError, error } = useMeshStatus()

  if (isPending) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>WireGuard mesh</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Loading mesh status...
          </p>
        </CardContent>
      </Card>
    )
  }

  if (isError) {
    // A 501 (mesh networking not enabled on this control plane) is the
    // overwhelmingly common case, not a failure to be alarmed about:
    // APP_MESH_ENABLED defaults off. Any other error still renders here,
    // just with the actual message instead of assuming "not enabled".
    return (
      <Card>
        <CardHeader>
          <CardTitle>WireGuard mesh</CardTitle>
        </CardHeader>
        <CardContent>
          <EmptyState
            icon={<WifiSlashIcon className="size-5" />}
            title="Mesh networking is not enabled"
            description={
              error instanceof ApiError && error.status === 501
                ? 'Set APP_MESH_ENABLED=1 on this control plane to bring up its WireGuard device and peer visibility.'
                : error.message
            }
          />
        </CardContent>
      </Card>
    )
  }

  const rotation = data.rotation

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <CardTitle>WireGuard mesh</CardTitle>
          {isLocal ? (
            <RotateMeshKeyDialog nodeId={nodeId} nodeName={nodeName} />
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div>
            <dt className="text-xs text-muted-foreground uppercase">Backend</dt>
            <dd className="mt-1 text-sm text-foreground">
              {data.backend || 'Unknown'}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground uppercase">
              Mesh address
            </dt>
            <dd className="mt-1 font-mono text-sm text-foreground">
              {data.mesh_address || 'Not assigned'}
            </dd>
          </div>
          <div className="col-span-2 min-w-0">
            <dt className="text-xs text-muted-foreground uppercase">
              Public key
            </dt>
            <dd className="mt-1 truncate font-mono text-sm text-foreground">
              {data.public_key || 'Not generated yet'}
            </dd>
          </div>
        </dl>

        {rotation ? (
          <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2 text-sm">
            {rotation.confirmed ? (
              <CheckCircleIcon
                className="size-4 text-green-600"
                aria-hidden="true"
              />
            ) : (
              <WarningCircleIcon
                className="size-4 text-amber-600"
                aria-hidden="true"
              />
            )}
            <span>
              Last rotation{' '}
              {rotation.confirmed ? 'confirmed' : 'still confirming'}: started{' '}
              {new Date(rotation.started_at).toLocaleString()}
              {rotation.confirmed && rotation.confirmed_at
                ? `, confirmed ${new Date(rotation.confirmed_at).toLocaleString()}`
                : ''}
            </span>
          </div>
        ) : null}

        {data.peers.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No peers configured yet.
          </p>
        ) : (
          <div className="overflow-hidden rounded-lg border border-border">
            <div
              className={`${PEER_LIST_GRID} border-b border-border bg-muted/40 px-3 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
            >
              <span>Peer</span>
              <span>Health</span>
              <span>Last handshake</span>
              <span className="text-right">Endpoint</span>
            </div>
            {data.peers.map((peer) => (
              <PeerRow key={peer.node_id || peer.public_key} peer={peer} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
