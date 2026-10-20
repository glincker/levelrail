import { PlugsConnectedIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRejoinNodeMesh } from '../queries/mesh'

// POST /api/v1/nodes/{id}/mesh/rejoin, unconfirmed unlike
// RotateMeshKeyDialog: forcing an extra reconcile pass is idempotent and
// non-destructive (internal/api/mesh.go's own doc comment on what
// "rejoin" actually does under the hood), the same "reversible, no data
// lost" reasoning DomainMaintenanceControl already uses to skip a confirm
// dialog for its own toggle.
export function RejoinMeshButton({
  nodeId,
  nodeName,
}: {
  nodeId: string
  nodeName: string
}) {
  const rejoin = useRejoinNodeMesh()

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={rejoin.isPending}
      onClick={() => {
        rejoin.mutate(nodeId, {
          onSuccess: (result) => {
            toast.add({
              title: `Mesh resync requested for "${nodeName}".`,
              description: result.requested
                ? 'The whole fleet reconciles right away, not just this node.'
                : 'No reconcile nudger is configured on this control plane; the fleet converges on its next scheduled pass instead.',
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: `Could not request a mesh rejoin for "${nodeName}".`,
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
      {rejoin.isPending ? 'Requesting...' : 'Rejoin'}
    </Button>
  )
}
