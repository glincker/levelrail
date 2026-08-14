import { useState } from 'react'
import {
  ArrowsOutIcon,
  CheckCircleIcon,
  WarningIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useDrainNode, useNodes } from '../queries/nodes'
import type { DrainNodeResponse, NodeResource } from '../types/nodeDetail'

// Local-node sentinel value: POST /api/v1/nodes/{id}/drain?target_node_id=
// treats an empty string as "the local/control-plane node"
// (handleDrainNode's own doc comment), but Base UI's Select cannot use
// an empty string as an item value (it reads as "no selection"), so this
// picks a stand-in that is translated back to '' right before the
// mutation fires.
const LOCAL_NODE_VALUE = '__local__'

// Drain action for the node detail route (routes/nodes/$id.tsx): moves
// every app and database placed on this node to a target node, mirroring
// DeleteNodeDialog/CordonNodeDialog's confirm-dialog shape but with an
// extra step, a target-node picker, and a real result screen instead of
// just closing on success. This is the consequential one of the three
// node actions (it moves real workloads, not just a flag flip), so the
// dialog does not auto-close on success: the operator has to read what
// moved and what didn't (drainNodeResponse's own partial-failure shape,
// internal/api/nodes.go) and explicitly dismiss it.
export function DrainNodeDialog({ node }: { node: NodeResource }) {
  const [open, setOpen] = useState(false)
  const [targetNodeId, setTargetNodeId] = useState<string>(LOCAL_NODE_VALUE)
  const [result, setResult] = useState<DrainNodeResponse | null>(null)
  const { data: nodes } = useNodes()
  const drain = useDrainNode()

  const otherNodes = nodes.filter((n) => n.id !== node.id)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      drain.reset()
      setResult(null)
      setTargetNodeId(LOCAL_NODE_VALUE)
    }
  }

  function handleDrain() {
    const resolvedTarget = targetNodeId === LOCAL_NODE_VALUE ? '' : targetNodeId
    drain.mutate(
      { id: node.id, targetNodeId: resolvedTarget },
      { onSuccess: setResult },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsOutIcon className="size-3.5" aria-hidden="true" />
        Drain
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {result ? (
          <DrainResult
            result={result}
            onClose={() => handleOpenChange(false)}
          />
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-1.5">
                <WarningIcon
                  className="size-4 text-amber-600"
                  aria-hidden="true"
                />
                Drain &ldquo;{node.name}&rdquo;?
              </DialogTitle>
              <DialogDescription>
                Moves every app and database currently placed on this node to
                the target below. Desired placement changes immediately, but
                each moved resource&apos;s container only actually relocates
                once the reconcile engine&apos;s next pass converges it on the
                new node.
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-1.5">
              <label
                htmlFor="drain-target-node"
                className="text-sm font-medium text-foreground"
              >
                Move workloads to
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
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={LOCAL_NODE_VALUE}>
                    This control plane (local)
                  </SelectItem>
                  {otherNodes.map((n) => (
                    <SelectItem
                      key={n.id}
                      value={n.id}
                      disabled={!n.schedulable}
                    >
                      {n.name}
                      {n.schedulable ? '' : ' (cordoned)'}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {drain.isError ? (
              <p className="text-sm text-destructive">{drain.error.message}</p>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                disabled={drain.isPending}
                onClick={handleDrain}
              >
                {drain.isPending ? 'Draining...' : 'Drain node'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

// Renders drainNodeResponse's actual partial-failure shape: what moved,
// what didn't, and why, instead of collapsing the request to pass/fail.
// A non-empty Errors list still shows MovedServices/MovedDatabases as
// genuinely successful, matching the backend's own 207-is-not-a-failure
// reasoning (internal/api/nodes.go's handleDrainNode doc comment).
function DrainResult({
  result,
  onClose,
}: {
  result: DrainNodeResponse
  onClose: () => void
}) {
  const hasErrors = (result.errors?.length ?? 0) > 0
  const movedNothing =
    result.moved_services.length === 0 && result.moved_databases.length === 0

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-1.5">
          {hasErrors ? (
            <Badge variant="warning">Partially drained</Badge>
          ) : (
            <Badge variant="success">Drained</Badge>
          )}
        </DialogTitle>
        <DialogDescription>
          {movedNothing && !hasErrors
            ? 'Nothing was placed on this node, so there was nothing to move.'
            : 'This is exactly what moved and what did not.'}
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-3">
        {result.moved_services.length > 0 ? (
          <ResultList
            icon={<CheckCircleIcon className="size-4 text-green-600" />}
            title="Apps moved"
            items={result.moved_services}
          />
        ) : null}
        {result.moved_databases.length > 0 ? (
          <ResultList
            icon={<CheckCircleIcon className="size-4 text-green-600" />}
            title="Databases moved"
            items={result.moved_databases}
          />
        ) : null}
        {hasErrors ? (
          <ResultList
            icon={<XCircleIcon className="size-4 text-destructive" />}
            title="Failed to move"
            items={result.errors ?? []}
            variant="destructive"
          />
        ) : null}
      </div>

      <DialogFooter>
        <Button type="button" onClick={onClose}>
          Close
        </Button>
      </DialogFooter>
    </>
  )
}

function ResultList({
  icon,
  title,
  items,
  variant = 'default',
}: {
  icon: React.ReactNode
  title: string
  items: string[]
  variant?: 'default' | 'destructive'
}) {
  return (
    <div>
      <p className="mb-1 flex items-center gap-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {icon}
        {title}
      </p>
      <ul
        className={`space-y-0.5 text-sm ${variant === 'destructive' ? 'text-destructive' : 'text-foreground'}`}
      >
        {items.map((item) => (
          <li key={item} className="truncate font-mono text-xs">
            {item}
          </li>
        ))}
      </ul>
    </div>
  )
}
