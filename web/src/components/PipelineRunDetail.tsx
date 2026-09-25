import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import {
  ArrowClockwiseIcon,
  StopIcon,
  ThumbsDownIcon,
  ThumbsUpIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  isTerminalStatus,
  useCancelPipelineRun,
  useDecidePipelineApproval,
  usePipelineRun,
  useRerunPipelineRun,
} from '../queries/pipelines'
import type { PipelineApproval, PipelineRun } from '../types/pipelines'
import { formatDuration } from '../lib/pipelineStatus'
import { PipelineRunGraph } from './PipelineRunGraph'
import { PipelineRunLogs } from './PipelineRunLogs'
import { PipelineStatusBadge } from './PipelineStatusBadge'
import { PipelineStepList } from './PipelineStepList'

function ApprovalGate({
  app,
  runId,
  approval,
}: {
  app: string
  runId: string
  approval: PipelineApproval
}) {
  const [comment, setComment] = useState('')
  const decide = useDecidePipelineApproval(app, runId)
  const act = (decision: 'approved' | 'rejected') =>
    decide.mutate(
      { approvalId: approval.id, decision, comment },
      { onError: (e) => toast.add({ title: e.message, type: 'error' }) },
    )
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-800 dark:bg-amber-950/30">
      <div className="min-w-48 flex-1">
        <p className="font-medium text-foreground">{approval.message}</p>
        <p className="text-xs text-muted-foreground">
          Job {approval.job} waits for someone with the{' '}
          {approval.required_ability} ability.
        </p>
      </div>
      <Input
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        placeholder="Comment (optional)"
        aria-label="Approval comment"
        className="w-48"
      />
      <Button
        size="sm"
        disabled={decide.isPending}
        onClick={() => act('approved')}
      >
        <ThumbsUpIcon aria-hidden="true" />
        Approve
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

function RunHeader({ app, run }: { app: string; run: PipelineRun }) {
  const cancel = useCancelPipelineRun(app)
  const rerun = useRerunPipelineRun(app)
  const navigate = useNavigate()
  const active = !isTerminalStatus(run.status)
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h2 className="flex flex-wrap items-center gap-2 text-base font-semibold text-foreground">
          {run.pipeline_name} #{run.number}
          <PipelineStatusBadge status={run.status} />
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {run.trigger}
          {run.actor ? ` by ${run.actor}` : ''}
          {run.ref ? ` on ${run.ref}` : ''}
          {run.commit_sha ? ` at ${run.commit_sha.slice(0, 8)}` : ''}
          {run.started_at
            ? ` (${formatDuration(run.started_at, run.finished_at)})`
            : ''}
        </p>
        {run.reason ? (
          <p className="mt-1 text-sm text-muted-foreground">{run.reason}</p>
        ) : null}
      </div>
      <div className="flex gap-2">
        {active ? (
          <Button
            variant="destructive"
            size="sm"
            disabled={cancel.isPending}
            onClick={() =>
              cancel.mutate(run.id, {
                onError: (e) => toast.add({ title: e.message, type: 'error' }),
              })
            }
          >
            <StopIcon aria-hidden="true" />
            Cancel
          </Button>
        ) : null}
        <Button
          variant="outline"
          size="sm"
          disabled={rerun.isPending}
          onClick={() =>
            rerun.mutate(run.id, {
              onSuccess: (next) =>
                void navigate({
                  to: '/apps/$name/pipelines/runs/$runId',
                  params: { name: app, runId: next.id },
                }),
              onError: (e) => toast.add({ title: e.message, type: 'error' }),
            })
          }
        >
          <ArrowClockwiseIcon aria-hidden="true" />
          Re-run
        </Button>
      </div>
    </div>
  )
}

export function PipelineRunDetail({
  app,
  runId,
  job: picked = '',
  step,
  onPick,
}: {
  app: string
  runId: string
  job?: string
  step?: number
  onPick: (job: string, step?: number) => void
}) {
  const { data: run, isLoading, error } = usePipelineRun(app, runId)

  if (isLoading) {
    return <Skeleton className="h-64 w-full" />
  }
  if (error || !run) {
    return (
      <p className="text-sm text-destructive">
        {error?.message ?? 'Run not found'}
      </p>
    )
  }
  const jobs = run.jobs ?? []
  const selectedKey =
    picked ||
    jobs.find((j) => j.status === 'failed')?.key ||
    jobs.find((j) => j.status === 'running')?.key ||
    jobs[0]?.key ||
    ''
  const selected = jobs.find((j) => j.key === selectedKey)
  const pending = (run.approvals ?? []).filter((a) => !a.decision)

  return (
    <section className="space-y-4 rounded-lg border border-border p-4">
      <p className="text-xs">
        <Link
          to="/apps/$name/pipelines"
          params={{ name: app }}
          className="text-muted-foreground underline underline-offset-2"
        >
          All pipelines
        </Link>
      </p>
      <RunHeader app={app} run={run} />
      {pending.map((a) => (
        <ApprovalGate key={a.id} app={app} runId={run.id} approval={a} />
      ))}
      {jobs.length > 0 ? (
        <PipelineRunGraph
          jobs={jobs}
          selected={selectedKey}
          onSelect={(key) => onPick(key)}
        />
      ) : (
        <p className="text-sm text-muted-foreground">Waiting to start.</p>
      )}
      {selected ? (
        <div className="space-y-3">
          <h3 className="flex flex-wrap items-center gap-2 text-sm font-semibold text-foreground">
            {selected.key}
            <PipelineStatusBadge status={selected.status} />
            {selected.started_at ? (
              <span className="text-xs font-normal tabular-nums text-muted-foreground">
                {formatDuration(selected.started_at, selected.finished_at)}
              </span>
            ) : null}
          </h3>
          <PipelineStepList
            job={selected}
            step={step}
            onPickStep={(next) => onPick(selected.key, next)}
          />
          <PipelineRunLogs
            app={app}
            runId={run.id}
            job={selected.key}
            step={step}
            live={!isTerminalStatus(run.status)}
          />
        </div>
      ) : null}
    </section>
  )
}
