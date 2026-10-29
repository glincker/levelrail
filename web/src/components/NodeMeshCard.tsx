import {
  RestoreGlassIcon,
  WifiHighGlassIcon,
  WifiOffGlassIcon,
} from '@thesvg/ui-react'
import { useMeshStatus } from '../queries/mesh'
import type { MeshPeerResource } from '../types/mesh'
import { ApiError } from '../lib/apiError'
import { RejoinMeshButton } from './RejoinMeshButton'
import { RotateMeshKeyDialog } from './RotateMeshKeyDialog'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
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

function handshakeAge(iso?: string): string {
  if (!iso) {
    return 'Never'
  }
  const secs = Math.max(0, Math.round((Date.now() - Date.parse(iso)) / 1000))
  if (secs < 90) {
    return `${secs}s ago`
  }
  if (secs < 5400) {
    return `${Math.round(secs / 60)} min ago`
  }
  return new Date(iso).toLocaleString()
}

function PeerRow({ peer }: { peer: MeshPeerResource }) {
  const label = peer.name || peer.node_id
  return (
    <div
      role="row"
      className={`${PEER_LIST_GRID} border-b border-border px-3 py-2 text-sm last:border-b-0`}
    >
      <div role="cell" className="min-w-0">
        <div className="truncate font-medium text-foreground">{label}</div>
        <div className="truncate font-mono text-xs text-muted-foreground">
          {peer.mesh_address || 'No mesh address yet'}
        </div>
      </div>
      <div role="cell">
        {!peer.live ? (
          <Badge variant="muted">
            <RestoreGlassIcon className="size-4.5" aria-hidden="true" />
            Not peered yet
          </Badge>
        ) : peer.healthy ? (
          <Badge variant="success">
            <WifiHighGlassIcon className="size-4.5" aria-hidden="true" />
            Healthy
          </Badge>
        ) : (
          <Badge variant="destructive">
            <WifiOffGlassIcon className="size-4.5" aria-hidden="true" />
            Stale
          </Badge>
        )}
      </div>
      <div role="cell" className="truncate text-xs text-muted-foreground">
        {peer.live
          ? formatHandshake(peer.last_handshake_at)
          : 'No live peer entry'}
      </div>
      <div
        role="cell"
        className="text-right font-mono text-xs text-muted-foreground"
      >
        {peer.endpoint || ''}
      </div>
    </div>
  )
}

// Node detail route's mesh section: the selected node's own mesh identity
// (the control plane's for the local node, the control plane's peer entry
// for a remote one), plus a rotate-key action. See
// internal/api/mesh.go's own doc comment: GET /api/v1/mesh is one live
// view (the control plane's own node's UAPI-backed status, plus
// best-effort static info for every other node), so this card renders
// the same content on every node's detail page. The rotate action is
// offered on every node now, local or remote: POST
// /api/v1/nodes/{id}/mesh/rotate-key reaches any currently-connected
// node over its own agent Session (internal/agent.GRPCSink). A node with
// no live connection still surfaces a real error from the dialog itself
// (RotateMeshKeyDialog renders rotate.error.message verbatim) rather than
// hiding the button, since "not currently connected" is exactly the kind
// of node state an operator opening this card wants to discover, not one
// this UI should mask by omission.
export function NodeMeshCard({
  nodeId,
  nodeName,
}: {
  nodeId: string
  nodeName: string
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
            icon={<WifiOffGlassIcon className="size-6" />}
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
  // The control plane only has its own device, so a remote node's own
  // identity is the control plane's peer entry for it, never the local keys.
  const isLocalNode = !nodeId || nodeId === data.local_node_id
  const self = isLocalNode
    ? undefined
    : data.peers.find((p) => p.node_id === nodeId)
  const peers = isLocalNode
    ? data.peers
    : data.peers.filter((p) => p.node_id === nodeId)
  const meshAddress = isLocalNode ? data.mesh_address : self?.mesh_address
  const publicKey = isLocalNode ? data.public_key : self?.public_key

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <CardTitle>WireGuard mesh</CardTitle>
          <div className="flex flex-wrap items-center gap-2">
            <RejoinMeshButton nodeId={nodeId} nodeName={nodeName} />
            <RotateMeshKeyDialog nodeId={nodeId} nodeName={nodeName} />
          </div>
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
            <dd className="mt-1 break-all font-mono text-sm text-foreground">
              {meshAddress || 'Not assigned'}
            </dd>
          </div>
          <div className="col-span-2 min-w-0">
            <dt className="text-xs text-muted-foreground uppercase">
              Public key
            </dt>
            <dd className="mt-1 truncate font-mono text-sm text-foreground">
              {publicKey || 'Not generated yet'}
            </dd>
          </div>
          {!isLocalNode ? (
            <>
              <div>
                <dt className="text-xs text-muted-foreground uppercase">
                  Endpoint
                </dt>
                <dd className="mt-1 break-all font-mono text-sm text-foreground">
                  {self?.endpoint || 'Unknown'}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground uppercase">
                  Last handshake
                </dt>
                <dd className="mt-1 text-sm text-foreground">
                  {handshakeAge(self?.last_handshake_at)}
                </dd>
              </div>
            </>
          ) : null}
        </dl>

        {rotation ? (
          <Banner
            variant={rotation.confirmed ? 'success' : 'warning'}
            title={`Last rotation ${rotation.confirmed ? 'confirmed' : 'still confirming'}: started ${new Date(rotation.started_at).toLocaleString()}${
              rotation.confirmed && rotation.confirmed_at
                ? `, confirmed ${new Date(rotation.confirmed_at).toLocaleString()}`
                : ''
            }`}
          />
        ) : null}

        {peers.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No peers configured yet.
          </p>
        ) : (
          <div
            role="table"
            aria-label="Mesh peers"
            className="overflow-hidden rounded-lg border border-border"
          >
            <div
              role="row"
              className={`${PEER_LIST_GRID} border-b border-border bg-muted/40 px-3 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
            >
              <span role="columnheader">Peer</span>
              <span role="columnheader">Health</span>
              <span role="columnheader">Last handshake</span>
              <span role="columnheader" className="text-right">
                Endpoint
              </span>
            </div>
            {peers.map((peer) => (
              <PeerRow key={peer.node_id || peer.public_key} peer={peer} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
