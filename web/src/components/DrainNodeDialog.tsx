import { useState } from 'react'
import type { DialogControl } from './dialogControl'
import { useTranslation } from 'react-i18next'
import { MaximizeGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
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
import { useNodes } from '../queries/nodes'
import { useNodeDrain, useStartNodeDrain } from '../queries/nodeMoves'
import type { NodeResource } from '../types/nodeDetail'
import { DrainStatusBadge, NodeDrainProgress } from './nodes/NodeDrainProgress'

// Empty string is not a valid Select item value in Base UI.
const AUTOMATIC_VALUE = '__auto__'

// Drain starts a background job: the dialog polls it and lists each move
// with its phase, so the operator can watch it or close it and come back
// through the node's Drains card.
export function DrainNodeDialog({
  node,
  control,
}: {
  node: NodeResource
  control?: DialogControl
}) {
  const { t } = useTranslation('nodes')
  const [internalOpen, setInternalOpen] = useState(false)
  const open = control?.open ?? internalOpen
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    control?.onOpenChange?.(next)
  }
  const [targetNodeId, setTargetNodeId] = useState(AUTOMATIC_VALUE)
  const [drainId, setDrainId] = useState<string | null>(null)
  const { data: nodes } = useNodes()
  const start = useStartNodeDrain()
  const drain = useNodeDrain(drainId)

  const otherNodes = nodes.filter((n) => n.id !== node.id)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      start.reset()
      setDrainId(null)
      setTargetNodeId(AUTOMATIC_VALUE)
    }
  }

  function handleDrain() {
    const target = targetNodeId === AUTOMATIC_VALUE ? '' : targetNodeId
    start.mutate(
      { id: node.id, targetNodeId: target },
      { onSuccess: (d) => setDrainId(d.id) },
    )
  }

  const current = drain.data
  const targetLabel =
    targetNodeId === AUTOMATIC_VALUE
      ? t('drain.automatic')
      : (otherNodes.find((n) => n.id === targetNodeId)?.name ?? targetNodeId)

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {control?.hideTrigger ? null : (
        <DialogTrigger
          render={<Button type="button" variant="outline" size="sm" />}
        >
          <MaximizeGlassIcon className="size-5" aria-hidden="true" />
          {t('drain.trigger')}
        </DialogTrigger>
      )}
      <DialogContent className="sm:max-w-lg">
        {drainId ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                {t('drain.title', { name: node.name })}
                {current ? <DrainStatusBadge status={current.status} /> : null}
              </DialogTitle>
              <DialogDescription>
                {current?.status === 'running' || !current
                  ? t('drain.runningHint')
                  : null}
              </DialogDescription>
            </DialogHeader>
            {drain.isError ? (
              <Alert variant="destructive">
                <TriangleAlertGlassIcon
                  className="text-tone-warning"
                  size="1.4em"
                />
                <AlertDescription>{drain.error.message}</AlertDescription>
              </Alert>
            ) : null}
            {current ? <NodeDrainProgress drain={current} /> : null}
            <DialogFooter>
              <Button type="button" onClick={() => handleOpenChange(false)}>
                {t('drain.close')}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-1.5">
                <TriangleAlertGlassIcon
                  className="size-5 text-amber-600"
                  aria-hidden="true"
                />
                {t('drain.title', { name: node.name })}
              </DialogTitle>
              <DialogDescription>{t('drain.description')}</DialogDescription>
            </DialogHeader>

            <div className="space-y-1.5">
              <label
                htmlFor="drain-target-node"
                className="text-sm font-medium text-foreground"
              >
                {t('drain.targetLabel')}
              </label>
              <Select
                value={targetNodeId}
                onValueChange={(value) => {
                  if (value) {
                    setTargetNodeId(value)
                  }
                }}
              >
                <SelectTrigger id="drain-target-node" className="w-full">
                  <SelectValue>{targetLabel}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={AUTOMATIC_VALUE}>
                    {t('drain.automatic')}
                  </SelectItem>
                  {otherNodes.map((n) => (
                    <SelectItem
                      key={n.id}
                      value={n.id}
                      disabled={!n.schedulable}
                    >
                      {n.name}
                      {n.schedulable ? '' : ` (${t('drain.cordoned')})`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {start.isError ? (
              <Alert variant="destructive">
                <TriangleAlertGlassIcon
                  className="text-tone-warning"
                  size="1.4em"
                />
                <AlertDescription>{start.error.message}</AlertDescription>
              </Alert>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => handleOpenChange(false)}
              >
                {t('drain.cancel')}
              </Button>
              <Button
                type="button"
                disabled={start.isPending}
                onClick={handleDrain}
              >
                {start.isPending ? t('drain.starting') : t('drain.start')}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
