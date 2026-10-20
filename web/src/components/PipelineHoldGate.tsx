import { ThumbsDownIcon, ThumbsUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDecidePipelineRunHold } from '../queries/pipelines'
import type { PipelineHold } from '../types/pipelines'

// A run held for approval, such as a pull request from a fork: no job has
// started and no secret has been read until an approver releases it.
export function PipelineHoldGate({
  app,
  runId,
  hold,
}: {
  app: string
  runId: string
  hold: PipelineHold
}) {
  const decide = useDecidePipelineRunHold(app, runId)
  const act = (decision: 'approved' | 'rejected') =>
    decide.mutate(decision, {
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
    })
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-800 dark:bg-amber-950/30"
    >
      <div className="min-w-48 flex-1">
        <p className="font-medium text-foreground">{hold.reason}</p>
        <p className="text-xs text-muted-foreground">
          Nothing has run and no secret has been read. Approve only if you have
          reviewed the pull request&apos;s changes.
        </p>
      </div>
      <Button
        size="sm"
        disabled={decide.isPending}
        onClick={() => act('approved')}
      >
        <ThumbsUpIcon aria-hidden="true" />
        Approve and run
      </Button>
      <Button
        size="sm"
        variant="destructive"
        disabled={decide.isPending}
        onClick={() => act('rejected')}
      >
        <ThumbsDownIcon aria-hidden="true" />
        Reject
      </Button>
    </div>
  )
}
