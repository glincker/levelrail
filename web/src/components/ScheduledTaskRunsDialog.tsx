import { useState } from 'react'
import { ClockCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import { Button } from '@/components/ui/button'
import { useScheduledTaskRuns } from '../queries/scheduledTasks'
import type { ScheduledTask, ScheduledTaskRunStatus } from '../types/scheduledTasks'

const STATUS_BADGE_VARIANT: Record<
  ScheduledTaskRunStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  running: 'warning',
  succeeded: 'success',
  failed: 'destructive',
}

function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

// Execution history for one scheduled task: both cron-triggered and
// manually-triggered runs share the same store.ScheduledTaskRun rows
// server-side, so this shows both without distinguishing which kind
// started a given run, the same "no separate manual/scheduled shape"
// reasoning internal/scheduledtask.Runner's own doc comment gives.
// Query only enabled while the dialog is open: no reason to keep polling
// or fetching history for a task the operator isn't currently looking at.
export function ScheduledTaskRunsDialog({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const [open, setOpen] = useState(false)
  const { data, isLoading, error } = useScheduledTaskRuns(appName, task.id, open)
  const runs = data ?? []

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <ClockCounterClockwiseIcon className="size-3.5" aria-hidden="true" />
        History
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Runs: {task.name}</DialogTitle>
          <DialogDescription className="font-mono text-xs">
            {task.command}
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : runs.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No runs yet. Runs appear here once the schedule fires or you trigger
            one manually.
          </p>
        ) : (
          <div className="max-h-[60vh] overflow-y-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Status</TableHead>
                  <TableHead>Exit code</TableHead>
                  <TableHead>Started</TableHead>
                  <TableHead>Finished</TableHead>
                  <TableHead>Output</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((run) => (
                  <TableRow key={run.id}>
                    <TableCell>
                      <Badge variant={STATUS_BADGE_VARIANT[run.status]}>
                        {run.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {run.status === 'running' ? '-' : run.exit_code}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatDate(run.started_at, '-')}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatDate(run.finished_at, '-')}
                    </TableCell>
                    <TableCell className="max-w-[20rem]">
                      <pre className="max-h-24 overflow-y-auto whitespace-pre-wrap break-all font-mono text-xs text-muted-foreground">
                        {run.error || run.output || '-'}
                      </pre>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
