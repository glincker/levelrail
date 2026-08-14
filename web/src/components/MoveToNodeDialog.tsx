import { useState } from 'react'
import {
  ArrowsLeftRightIcon,
  WarningIcon,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useSetAppNode } from '../queries/apps'
import { useSetDatabaseNode } from '../queries/databases'
import { useNodeListOptional } from '../queries/nodes'

// Local-node sentinel, mirrors DrainNodeDialog's own: Base UI's Select
// can't use an empty string as an item value (reads as "no selection"),
// but both PUT .../node endpoints treat an empty node_id as "this
// control plane, local" (setAppNode/setDatabaseNode's own doc comments
// in queries/apps.ts and queries/databases.ts).
const LOCAL_NODE_VALUE = '__local__'

type MoveToNodeKind = 'app' | 'database'

// One dialog shared by both the app and database Overview sections
// rather than two near-identical components. useSetAppNode and
// useSetDatabaseNode differ only in which PUT .../node endpoint they
// call: both take the same {name, nodeId} mutate shape, and the
// meaningful behavioral difference (handleSetAppNode rejects a cordoned
// target with a 400, handleSetDatabaseNode's own doc comment says it
// only checks that the node exists, not that it's schedulable) is a
// backend-side detail this dialog doesn't need to special-case, the
// error message either endpoint returns is surfaced verbatim either
// way. Both mutation hooks are called unconditionally so hook order
// stays stable across renders (kind is fixed per mount, never toggled),
// then the right one is picked by `kind`.
export function MoveToNodeDialog({
  kind,
  name,
  currentNodeId,
}: {
  kind: MoveToNodeKind
  name: string
  currentNodeId?: string
}) {
  const [open, setOpen] = useState(false)
  const [targetNodeId, setTargetNodeId] = useState(LOCAL_NODE_VALUE)
  const setAppNode = useSetAppNode()
  const setDatabaseNode = useSetDatabaseNode()
  const mutation = kind === 'app' ? setAppNode : setDatabaseNode
  // Optional convenience only, see useNodeListOptional's own doc
  // comment: this dialog's whole purpose depends on there being other
  // nodes to move to, but a failure to list them must degrade to "no
  // other nodes available" rather than crash the Overview page this
  // trigger lives on.
  const nodeList = useNodeListOptional()
  const nodes = nodeList.data ?? []
  const otherNodes = nodes.filter((n) => n.id !== currentNodeId)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      // Pre-select the resource's actual current placement so the
      // dialog reads as "here's where it is, pick somewhere else"
      // rather than always defaulting back to local.
      setTargetNodeId(currentNodeId || LOCAL_NODE_VALUE)
    } else {
      setAppNode.reset()
      setDatabaseNode.reset()
    }
  }

  const resolvedTarget = targetNodeId === LOCAL_NODE_VALUE ? '' : targetNodeId
  const isNoop = (currentNodeId ?? '') === resolvedTarget
  const hasNoOtherNodes = otherNodes.length === 0

  function handleMove() {
    mutation.mutate(
      { name, nodeId: resolvedTarget },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: `${kind === 'app' ? 'App' : 'Database'} "${name}" moved.`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsLeftRightIcon className="size-3.5" aria-hidden="true" />
        Move
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Move &ldquo;{name}&rdquo; to another node</DialogTitle>
          <DialogDescription>
            Desired placement changes immediately, but the container only
            actually relocates once the reconcile engine&apos;s next pass
            converges it on the new node.
          </DialogDescription>
        </DialogHeader>

        {hasNoOtherNodes ? (
          <p className="text-sm text-muted-foreground">
            No other nodes available to move to.
          </p>
        ) : (
          <div className="space-y-1.5">
            <label
              htmlFor="move-target-node"
              className="text-sm font-medium text-foreground"
            >
              Move to
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
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={LOCAL_NODE_VALUE}>
                  This control plane (local)
                  {currentNodeId ? '' : ' (current)'}
                </SelectItem>
                {otherNodes.map((n) => (
                  <SelectItem key={n.id} value={n.id} disabled={!n.schedulable}>
                    {n.name}
                    {n.schedulable ? '' : ' (cordoned)'}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {mutation.isError ? (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {mutation.error.message}
          </p>
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
            disabled={mutation.isPending || hasNoOtherNodes || isNoop}
            onClick={handleMove}
          >
            {mutation.isPending ? 'Moving...' : 'Move'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
