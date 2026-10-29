import { useTranslation } from 'react-i18next'
import { ServerGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { useSetAppNode } from '../queries/apps'
import { useSetDatabaseNode } from '../queries/databases'
import {
  resolvePlacement,
  type PlacementInput,
  type PlacementNode,
} from '../lib/placement'
import type { NodeMoveKind } from '../types/nodeMoves'

interface PlacementLabelProps extends PlacementInput {
  nodes?: readonly PlacementNode[]
  className?: string
}

// Where a resource runs: "This server" (plus its hostname) for the control
// plane's own node, the node name for a remote one, and "Unassigned" only
// for a placement that points at a node that no longer exists.
export function PlacementLabel({
  nodeId,
  nodeName,
  nodeIsLocal,
  nodes,
  className,
}: PlacementLabelProps) {
  const { t } = useTranslation('nodes')
  const placement = resolvePlacement({ nodeId, nodeName, nodeIsLocal }, nodes)

  if (placement.kind === 'unknown') {
    return (
      <span
        className={cn('text-muted-foreground italic', className)}
        title={t('placement.unknownHint', { id: placement.name })}
      >
        {t('placement.unassigned')}
      </span>
    )
  }
  if (placement.kind === 'local') {
    return (
      <span className={cn('min-w-0 truncate', className)}>
        <span className="text-foreground">{t('placement.thisServer')}</span>
        {placement.name ? (
          <span className="ml-1.5 text-muted-foreground">{placement.name}</span>
        ) : null}
      </span>
    )
  }
  return (
    <span className={cn('min-w-0 truncate text-foreground', className)}>
      {placement.name}
    </span>
  )
}

// Fix action for an unknown placement: points the resource back at this
// server. Renders nothing when the placement already resolves.
export function MoveToThisServerButton({
  kind,
  name,
  nodeId,
  nodeName,
  nodeIsLocal,
  nodes,
}: PlacementInput & {
  kind: NodeMoveKind
  name: string
  nodes?: readonly PlacementNode[]
}) {
  const { t } = useTranslation('nodes')
  const setApp = useSetAppNode()
  const setDatabase = useSetDatabaseNode()
  const mutation = kind === 'app' ? setApp : setDatabase
  const placement = resolvePlacement({ nodeId, nodeName, nodeIsLocal }, nodes)
  if (placement.kind !== 'unknown') {
    return null
  }
  return (
    <Button
      type="button"
      size="xs"
      variant="outline"
      disabled={mutation.isPending}
      onClick={() => {
        mutation.mutate(
          { name, nodeId: '' },
          {
            onSuccess: () => {
              toast.add({ title: t('placement.movedHere'), type: 'success' })
            },
            onError: (err) => {
              toast.add({
                title: t('placement.moveFailed'),
                description: err.message,
                type: 'error',
              })
            },
          },
        )
      }}
    >
      <ServerGlassIcon size="1.2em" aria-hidden="true" />
      {t('placement.moveHere')}
    </Button>
  )
}
