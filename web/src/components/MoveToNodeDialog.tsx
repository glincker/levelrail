import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftRightGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useNodeListOptional } from '../queries/nodes'
import { useNodeMove, useStartNodeMove } from '../queries/nodeMoves'
import type { NodeResource } from '../types/nodeDetail'
import { MoveStatusBadge, NodeMoveSteps } from './nodes/NodeMoveSteps'

// Empty string is not a valid Select item value in Base UI.
const AUTOMATIC_VALUE = '__auto__'

type MoveToNodeKind = 'app' | 'database'

function effectiveCurrentNode(
  nodes: NodeResource[],
  currentNodeId: string | undefined,
): string {
  if (currentNodeId) {
    return currentNodeId
  }
  return nodes.find((n) => n.is_local)?.id ?? ''
}

// One dialog for both app and database overviews: starts a verified move
// (POST .../move), then polls the move and shows each phase as it runs.
export function MoveToNodeDialog({
  kind,
  name,
  currentNodeId,
  volumeNames = [],
}: {
  kind: MoveToNodeKind
  name: string
  currentNodeId?: string
  volumeCount?: number
  volumeNames?: string[]
}) {
  const { t } = useTranslation('nodes')
  const [open, setOpen] = useState(false)
  const [targetNodeId, setTargetNodeId] = useState(AUTOMATIC_VALUE)
  const [moveId, setMoveId] = useState<string | null>(null)
  const start = useStartNodeMove(kind, name)
  const move = useNodeMove(moveId)

  // Optional convenience: a failed node list degrades to "no targets".
  const nodeList = useNodeListOptional()
  const nodes = nodeList.data ?? []
  const currentId = effectiveCurrentNode(nodes, currentNodeId)
  const targets = nodes.filter((n) => n.id !== currentId)

  const status = move.data?.status
  const running =
    start.isPending || (moveId !== null && (!status || status === 'running'))
  const failed = status === 'failed' || status === 'rolled_back'

  useEffect(() => {
    if (status === 'succeeded') {
      toast.add({ title: t('move.toastSucceeded', { name }), type: 'success' })
    }
  }, [status, name, t])

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      start.reset()
      setMoveId(null)
      setTargetNodeId(AUTOMATIC_VALUE)
    }
  }

  function handleMove() {
    const target = targetNodeId === AUTOMATIC_VALUE ? '' : targetNodeId
    start.mutate(target, { onSuccess: (m) => setMoveId(m.id) })
  }

  function optionLabel(n: NodeResource): string {
    const notes: string[] = []
    if (n.is_local) {
      notes.push(t('move.controlPlane'))
    }
    if (!n.schedulable) {
      notes.push(t('move.cordoned'))
    } else if (n.status === 'offline') {
      notes.push(t('move.offline'))
    }
    return notes.length === 0 ? n.name : `${n.name} (${notes.join(', ')})`
  }

  const selectedLabel =
    targetNodeId === AUTOMATIC_VALUE
      ? t('move.automatic')
      : (targets.find((n) => n.id === targetNodeId)?.name ?? targetNodeId)

  const noTargets = nodeList.isSuccess && targets.length === 0

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowLeftRightGlassIcon className="size-5" aria-hidden="true" />
        {t('move.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('move.title', { name })}</DialogTitle>
          <DialogDescription>
            {kind === 'app'
              ? t('move.descriptionApp')
              : t('move.descriptionDatabase')}
          </DialogDescription>
        </DialogHeader>

        {moveId === null ? (
          <>
            {noTargets ? (
              <p className="text-sm text-muted-foreground">
                {t('move.noTargets')}
              </p>
            ) : (
              <div className="space-y-1.5">
                <label
                  htmlFor="move-target-node"
                  className="text-sm font-medium text-foreground"
                >
                  {t('move.targetLabel')}
                </label>
                <Select
                  value={targetNodeId}
                  onValueChange={(value) => {
                    if (value) {
                      setTargetNodeId(value)
                    }
                  }}
                >
                  <SelectTrigger id="move-target-node" className="w-full">
                    <SelectValue>{selectedLabel}</SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={AUTOMATIC_VALUE}>
                      {t('move.automatic')}
                    </SelectItem>
                    {targets.map((n) => (
                      <SelectItem
                        key={n.id}
                        value={n.id}
                        disabled={!n.schedulable || n.status === 'offline'}
                      >
                        {optionLabel(n)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}

            {volumeNames.length > 0 ? (
              <p className="flex items-start gap-1.5 text-sm text-muted-foreground">
                <TriangleAlertGlassIcon
                  className="mt-0.5 size-6 shrink-0 text-amber-500"
                  aria-hidden="true"
                />
                <span>
                  {t('move.statefulWarning', {
                    name,
                    volumes: volumeNames.join(', '),
                  })}
                </span>
              </p>
            ) : kind === 'app' ? (
              <p className="text-sm text-muted-foreground">
                {t('move.statelessNote', { name })}
              </p>
            ) : null}
          </>
        ) : (
          <div className="space-y-2 rounded-md border border-border p-2">
            {move.data ? (
              <>
                <MoveStatusBadge status={move.data.status} />
                <NodeMoveSteps move={move.data} />
              </>
            ) : (
              <p className="text-xs text-muted-foreground">
                {t('move.moving')}
              </p>
            )}
          </div>
        )}

        {start.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>{start.error.message}</AlertDescription>
          </Alert>
        ) : null}

        {move.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>{move.error.message}</AlertDescription>
          </Alert>
        ) : null}

        {failed ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>
              <p>{move.data?.error || t('move.failedFallback')}</p>
              <p>
                {status === 'rolled_back'
                  ? t('move.rolledBackHint', { name })
                  : t('move.failedHint')}
              </p>
            </AlertDescription>
          </Alert>
        ) : null}

        <DialogFooter>
          {status === 'succeeded' || failed ? (
            <Button type="button" onClick={() => handleOpenChange(false)}>
              {status === 'succeeded' ? t('move.done') : t('move.close')}
            </Button>
          ) : (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => handleOpenChange(false)}
              >
                {running && moveId !== null
                  ? t('move.close')
                  : t('move.cancel')}
              </Button>
              {moveId === null ? (
                <Button
                  type="button"
                  disabled={start.isPending || noTargets}
                  onClick={handleMove}
                >
                  {start.isPending ? t('move.starting') : t('move.start')}
                </Button>
              ) : null}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
