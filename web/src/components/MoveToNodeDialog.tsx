import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  ArrowsLeftRightIcon,
  CheckCircleIcon,
  CircleNotchIcon,
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
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { appKeys, useSetAppNode } from '../queries/apps'
import { useSetDatabaseNode } from '../queries/databases'
import { useNodeListOptional } from '../queries/nodes'
import {
  useAppVolumeMove,
  useMoveAppWithVolumes,
} from '../queries/appVolumeMove'
import type { AppVolumeMoveStatus } from '../types/appVolumeMove'

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
// then the right one is picked by `kind`. kind="app" gets one more
// option, "take its volumes with it" (POST .../move-with-volumes instead
// of PUT .../node), shown only when volumeCount > 0.

// stepStatusIcon renders a small status glyph for one AppVolumeMoveStep,
// the live-progress list shown while a "move with volumes" is running.
function stepStatusIcon(status: AppVolumeMoveStatus) {
  switch (status) {
    case 'succeeded':
      return (
        <CheckCircleIcon
          className="size-3.5 shrink-0 text-primary"
          weight="fill"
          aria-hidden="true"
        />
      )
    case 'failed':
      return (
        <XCircleIcon
          className="size-3.5 shrink-0 text-destructive"
          weight="fill"
          aria-hidden="true"
        />
      )
    default:
      return (
        <CircleNotchIcon
          className="size-3.5 shrink-0 animate-spin text-muted-foreground"
          aria-hidden="true"
        />
      )
  }
}

export function MoveToNodeDialog({
  kind,
  name,
  currentNodeId,
  volumeCount = 0,
}: {
  kind: MoveToNodeKind
  name: string
  currentNodeId?: string
  // volumeCount is app.volumes?.length: only meaningful for kind="app",
  // it's what decides whether the "take volumes with it" option even
  // renders. A database's own volume lives entirely inside its managed
  // container and has no equivalent move path yet, so this is never
  // passed for kind="database".
  volumeCount?: number
}) {
  const [open, setOpen] = useState(false)
  const [targetNodeId, setTargetNodeId] = useState(LOCAL_NODE_VALUE)
  const [withVolumes, setWithVolumes] = useState(false)
  const [moveId, setMoveId] = useState<string | null>(null)
  const setAppNode = useSetAppNode()
  const setDatabaseNode = useSetDatabaseNode()
  const mutation = kind === 'app' ? setAppNode : setDatabaseNode
  // Called unconditionally for the same hook-order-stability reason
  // setAppNode/setDatabaseNode both are above: harmless for kind=
  // "database", which never sets withVolumes true (the checkbox that
  // would only never renders for it).
  const moveWithVolumes = useMoveAppWithVolumes(name)
  const appMove = useAppVolumeMove(name, moveId)
  const queryClient = useQueryClient()
  const showWithVolumesOption = kind === 'app' && volumeCount > 0

  // Watches a triggered move-with-volumes attempt to its conclusion:
  // handleMove's own onSuccess only sees the initial 202/200 response
  // (Status "running" or already "succeeded"), this is what notices a
  // background move actually finishing and reacts to it. Deliberately
  // does not close the dialog itself (react-hooks/set-state-in-effect):
  // the render below shows a "Done" button once succeededMove is set,
  // and closing happens from that real click handler instead. A failure
  // is left visible inline (the steps list below), not swallowed into a
  // toast, since a partially-applied volume move is exactly the kind of
  // thing CLAUDE.md's own "the main risk" section says an operator needs
  // to be able to diagnose.
  useEffect(() => {
    if (!moveId || appMove.data?.status !== 'succeeded') {
      return
    }
    toast.add({
      title: `App "${name}" moved, volumes included.`,
      type: 'success',
    })
    void queryClient.invalidateQueries({ queryKey: appKeys.detail(name) })
    void queryClient.invalidateQueries({ queryKey: appKeys.list() })
  }, [appMove.data, moveId, name, queryClient])

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
      moveWithVolumes.reset()
      setWithVolumes(false)
      setMoveId(null)
    }
  }

  const resolvedTarget = targetNodeId === LOCAL_NODE_VALUE ? '' : targetNodeId
  const isNoop = (currentNodeId ?? '') === resolvedTarget
  const hasNoOtherNodes = otherNodes.length === 0
  // A move-with-volumes attempt is "in flight" from the moment the
  // trigger request is sent until GetAppVolumeMove reports it's no
  // longer running: the synchronous no-volumes/same-node shortcut on the
  // server returns an already-"succeeded" record on the very first
  // response, so this only ever actually holds the dialog open+disabled
  // for a real volume copy.
  const movingWithVolumes =
    moveWithVolumes.isPending ||
    (moveId !== null && appMove.data?.status === 'running')
  const failedMove =
    moveId !== null && appMove.data?.status === 'failed' ? appMove.data : null
  const succeededMove =
    moveId !== null && appMove.data?.status === 'succeeded'
      ? appMove.data
      : null

  function handleMove() {
    if (kind === 'app' && withVolumes) {
      moveWithVolumes.mutate(
        { node_id: resolvedTarget },
        { onSuccess: (move) => setMoveId(move.id) },
      )
      return
    }
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

        {showWithVolumesOption ? (
          <div className="space-y-1.5">
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={withVolumes}
                disabled={movingWithVolumes}
                onCheckedChange={(checked) => {
                  setWithVolumes(checked === true)
                }}
              />
              Take {volumeCount === 1 ? 'its volume' : 'its volumes'} with it
            </label>
            {withVolumes ? (
              <p className="flex items-start gap-1.5 text-sm text-muted-foreground">
                <WarningIcon
                  className="mt-0.5 size-4 shrink-0 text-amber-500"
                  aria-hidden="true"
                />
                <span>
                  {name} will be stopped for the duration: each volume is
                  archived from its current node and restored onto the
                  destination before the app starts again there.
                </span>
              </p>
            ) : null}
          </div>
        ) : null}

        {moveId && appMove.data ? (
          <div className="space-y-1 rounded-md border border-border p-2">
            <p className="text-xs font-medium text-muted-foreground uppercase">
              {appMove.data.status === 'succeeded'
                ? 'Move complete'
                : appMove.data.status === 'failed'
                  ? 'Move failed'
                  : 'Moving...'}
            </p>
            <ul className="space-y-1">
              {appMove.data.steps.map((step) => (
                <li
                  key={step.name}
                  className="flex items-center gap-1.5 font-mono text-xs text-foreground"
                >
                  {stepStatusIcon(step.status)}
                  {step.name}
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        {mutation.isError ? (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {mutation.error.message}
          </p>
        ) : null}

        {moveWithVolumes.isError ? (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {moveWithVolumes.error.message}
          </p>
        ) : null}

        {failedMove ? (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {failedMove.error || 'The move failed partway through.'}
          </p>
        ) : null}

        <DialogFooter>
          {succeededMove ? (
            <Button
              type="button"
              onClick={() => {
                handleOpenChange(false)
              }}
            >
              Done
            </Button>
          ) : (
            <>
              <Button
                type="button"
                variant="outline"
                disabled={movingWithVolumes}
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                {failedMove ? 'Close' : 'Cancel'}
              </Button>
              <Button
                type="button"
                disabled={
                  mutation.isPending ||
                  movingWithVolumes ||
                  hasNoOtherNodes ||
                  isNoop ||
                  failedMove !== null
                }
                onClick={handleMove}
              >
                {mutation.isPending || movingWithVolumes ? 'Moving...' : 'Move'}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
