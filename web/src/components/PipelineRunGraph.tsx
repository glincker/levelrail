import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr'
import type { PipelineJob } from '../types/pipelines'
import { formatDuration, layoutJobs } from '../lib/pipelineStatus'
import { cn } from '@/lib/utils'
import { PipelineStatusIcon } from './PipelineStatusBadge'

const NODE_TONE: Record<string, string> = {
  succeeded: 'border-green-300 dark:border-green-800',
  failed: 'border-destructive/50',
  running: 'border-primary/60',
  waiting_approval: 'border-amber-300 dark:border-amber-700',
}

// The run as columns by dependency depth: each column starts once the one
// to its left is done, jobs in one column run in parallel. A job lists what
// it needs so the edges stay readable without drawing lines.
export function PipelineRunGraph({
  jobs,
  selected,
  onSelect,
}: {
  jobs: PipelineJob[]
  selected: string
  onSelect: (key: string) => void
}) {
  const columns = layoutJobs(jobs)
  return (
    <div
      className="flex items-stretch gap-2 overflow-x-auto pb-2"
      role="list"
      aria-label="Pipeline jobs by stage"
    >
      {columns.map((col, i) => (
        <div key={i} className="flex items-stretch gap-2" role="listitem">
          {i > 0 ? (
            <ArrowRightIcon
              className="mt-8 size-4 shrink-0 self-start text-muted-foreground"
              aria-hidden="true"
            />
          ) : null}
          <div className="flex w-52 shrink-0 flex-col gap-2">
            {col.map((job) => (
              <button
                key={job.key}
                type="button"
                onClick={() => onSelect(job.key)}
                aria-pressed={selected === job.key}
                className={cn(
                  'rounded-lg border bg-card p-2.5 text-left text-sm transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                  NODE_TONE[job.status] ?? 'border-border',
                  selected === job.key && 'ring-2 ring-ring',
                )}
              >
                <div className="flex items-center gap-1.5">
                  <PipelineStatusIcon
                    status={job.status}
                    className="size-4 shrink-0"
                  />
                  <span className="truncate font-medium text-foreground">
                    {job.key}
                  </span>
                </div>
                <div className="mt-1 flex justify-between gap-2 text-xs text-muted-foreground">
                  <span className="truncate">
                    {job.needs.length > 0
                      ? `needs ${job.needs.join(', ')}`
                      : 'no dependencies'}
                  </span>
                  <span className="shrink-0 tabular-nums">
                    {formatDuration(job.started_at, job.finished_at)}
                  </span>
                </div>
                {job.reason && job.status !== 'succeeded' ? (
                  <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                    {job.reason}
                  </p>
                ) : null}
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
