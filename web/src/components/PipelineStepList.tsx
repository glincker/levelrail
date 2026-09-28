import { LinkSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import type { PipelineJob } from '../types/pipelines'
import { formatDuration, stepLink } from '../lib/pipelineStatus'
import { PipelineStatusIcon } from './PipelineStatusBadge'

async function copyStepLink(job: string, step?: number) {
  try {
    await navigator.clipboard.writeText(
      stepLink(window.location.href, job, step),
    )
    toast.add({ title: 'Link copied', type: 'success' })
  } catch {
    toast.add({ title: 'Could not copy the link', type: 'error' })
  }
}

// One row per step with its status and duration. Picking a row narrows the
// log below to that step; picking it again shows the whole job.
export function PipelineStepList({
  job,
  step,
  onPickStep,
}: {
  job: PipelineJob
  step?: number
  onPickStep: (step?: number) => void
}) {
  return (
    <ol className="divide-y divide-border rounded-lg border border-border text-sm">
      {job.steps.map((s) => {
        const on = step === s.index
        return (
          <li key={s.index} className="flex items-center">
            <button
              type="button"
              aria-pressed={on}
              onClick={() => onPickStep(on ? undefined : s.index)}
              className={cn(
                'flex min-w-0 flex-1 items-center gap-2 px-3 py-1.5 text-left transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                on && 'bg-primary/10',
              )}
            >
              <PipelineStatusIcon
                status={s.status}
                className="size-4 shrink-0"
              />
              <span className="truncate text-foreground">{s.name}</span>
              <span className="text-xs text-muted-foreground">{s.kind}</span>
              {s.reason ? (
                <span className="truncate text-xs text-muted-foreground">
                  {s.reason}
                </span>
              ) : null}
              <span className="ml-auto shrink-0 text-xs tabular-nums text-muted-foreground">
                {formatDuration(s.started_at, s.finished_at)}
              </span>
            </button>
            <button
              type="button"
              aria-label={`Copy link to step ${s.name}`}
              onClick={() => void copyStepLink(job.key, s.index)}
              className="mr-1 rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <LinkSimpleIcon className="size-4" aria-hidden="true" />
            </button>
          </li>
        )
      })}
    </ol>
  )
}
