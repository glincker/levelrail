import {
  RestoreGlassIcon,
  WifiHighGlassIcon,
  WifiOffGlassIcon,
} from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { useMeshStatus } from '../queries/mesh'
import type { MeshPeerResource } from '../types/mesh'
import { ApiError } from '../lib/apiError'
import { RejoinMeshButton } from './RejoinMeshButton'
import { RotateMeshKeyDialog } from './RotateMeshKeyDialog'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

function formatHandshake(t: TFunction<'statePlaceholders'>, iso?: string) {
  if (!iso) {
    return t('mesh.never')
  }
  return new Date(iso).toLocaleString()
}

function handshakeAge(t: TFunction<'statePlaceholders'>, iso?: string) {
  if (!iso) {
    return t('mesh.never')
  }
  const secs = Math.max(0, Math.round((Date.now() - Date.parse(iso)) / 1000))
  if (secs < 90) {
    return t('mesh.secondsAgo', { count: secs })
  }
  if (secs < 5400) {
    return t('mesh.minutesAgo', { count: Math.round(secs / 60) })
  }
  return new Date(iso).toLocaleString()
}

function PeerRow({ peer }: { peer: MeshPeerResource }) {
  const { t } = useTranslation('statePlaceholders')
  const label = peer.name || peer.node_id
  return (
    <TableRow>
      <TableCell className="max-w-0 min-w-40">
        <div className="truncate font-medium text-foreground">{label}</div>
        <div className="truncate font-mono text-xs text-muted-foreground">
          {peer.mesh_address || t('mesh.noMeshAddress')}
        </div>
      </TableCell>
      <TableCell>
        {!peer.live ? (
          <Badge variant="muted">
            <RestoreGlassIcon className="size-4.5" aria-hidden="true" />
            {t('mesh.notPeered')}
          </Badge>
        ) : peer.healthy ? (
          <Badge variant="success">
            <WifiHighGlassIcon className="size-4.5" aria-hidden="true" />
            {t('mesh.healthy')}
          </Badge>
        ) : (
          <Badge variant="destructive">
            <WifiOffGlassIcon className="size-4.5" aria-hidden="true" />
            {t('mesh.stale')}
          </Badge>
        )}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {peer.live
          ? formatHandshake(t, peer.last_handshake_at)
          : t('mesh.noLivePeer')}
      </TableCell>
      <TableCell className="text-right font-mono text-xs text-muted-foreground">
        {peer.endpoint || ''}
      </TableCell>
    </TableRow>
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
  const { t } = useTranslation('statePlaceholders')
  const { data, isPending, isError, error } = useMeshStatus()

  if (isPending) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('mesh.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{t('mesh.loading')}</p>
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
          <CardTitle>{t('mesh.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <EmptyState
            icon={<WifiOffGlassIcon className="size-6" />}
            title={t('mesh.disabledTitle')}
            description={
              error instanceof ApiError && error.status === 501
                ? t('mesh.disabledHint')
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
          <CardTitle>{t('mesh.title')}</CardTitle>
          <div className="flex flex-wrap items-center gap-2">
            <RejoinMeshButton nodeId={nodeId} nodeName={nodeName} />
            <RotateMeshKeyDialog nodeId={nodeId} nodeName={nodeName} />
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <div>
            <dt className="text-xs text-muted-foreground">
              {t('mesh.backend')}
            </dt>
            <dd className="mt-1 text-sm text-foreground">
              {data.backend || t('mesh.unknown')}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">
              {t('mesh.meshAddress')}
            </dt>
            <dd className="mt-1 break-all font-mono text-sm text-foreground">
              {meshAddress || t('mesh.notAssigned')}
            </dd>
          </div>
          <div className="col-span-2 min-w-0">
            <dt className="text-xs text-muted-foreground">
              {t('mesh.publicKey')}
            </dt>
            <dd className="mt-1 truncate font-mono text-sm text-foreground">
              {publicKey || t('mesh.notGenerated')}
            </dd>
          </div>
          {!isLocalNode ? (
            <>
              <div>
                <dt className="text-xs text-muted-foreground">
                  {t('mesh.endpoint')}
                </dt>
                <dd className="mt-1 break-all font-mono text-sm text-foreground">
                  {self?.endpoint || t('mesh.unknown')}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground">
                  {t('mesh.lastHandshake')}
                </dt>
                <dd className="mt-1 text-sm text-foreground">
                  {handshakeAge(t, self?.last_handshake_at)}
                </dd>
              </div>
            </>
          ) : null}
        </dl>

        {rotation ? (
          <Banner
            variant={rotation.confirmed ? 'success' : 'warning'}
            title={
              rotation.confirmed
                ? rotation.confirmed_at
                  ? t('mesh.rotationConfirmed', {
                      started: new Date(rotation.started_at).toLocaleString(),
                      confirmed: new Date(
                        rotation.confirmed_at,
                      ).toLocaleString(),
                    })
                  : t('mesh.rotationConfirmedNoTime', {
                      started: new Date(rotation.started_at).toLocaleString(),
                    })
                : t('mesh.rotationPending', {
                    started: new Date(rotation.started_at).toLocaleString(),
                  })
            }
          />
        ) : null}

        {peers.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('mesh.noPeers')}</p>
        ) : (
          <div className="overflow-hidden rounded-lg border border-border">
            <Table aria-label={t('mesh.peersLabel')}>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('mesh.peer')}</TableHead>
                  <TableHead>{t('mesh.health')}</TableHead>
                  <TableHead>{t('mesh.lastHandshake')}</TableHead>
                  <TableHead className="text-right">
                    {t('mesh.endpoint')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {peers.map((peer) => (
                  <PeerRow key={peer.node_id || peer.public_key} peer={peer} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
