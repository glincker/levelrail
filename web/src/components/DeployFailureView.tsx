import { useId, useState } from 'react'
import {
  CaretRightIcon,
  LightbulbIcon,
  ArrowsClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { DeployFailure } from '../types/deployFailure'
import { Skeleton } from '@/components/ui/skeleton'

interface DeployFailureViewProps {
  failure?: DeployFailure
  loading?: boolean
  /** Shown instead of nothing when the attempt has no failure. */
  showEmpty?: boolean
}

/** DeployFailureView renders a deploy attempt's structured failure. */
export function DeployFailureView({
  failure,
  loading = false,
  showEmpty = false,
}: DeployFailureViewProps) {
  const [open, setOpen] = useState(false)
  const excerptId = useId()

  if (loading) {
    return (
      <div
        role="status"
        aria-busy="true"
        aria-label="Loading failure details"
        className="mt-2 space-y-2"
      >
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
      </div>
    )
  }

  if (!failure) {
    return showEmpty ? (
      <p className="mt-2 text-xs text-muted-foreground">
        No failure recorded for this deploy.
      </p>
    ) : null
  }

  return (
    <section
      aria-label="Deploy failure"
      className="mt-2 space-y-2 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm"
    >
      <p className="text-destructive">
        <span className="font-medium">{failure.cause}</span>
        {failure.failing_step ? (
          <span className="text-muted-foreground">
            {' '}
            (failed at {failure.failing_step})
          </span>
        ) : null}
      </p>
      <p className="flex items-start gap-1.5 text-foreground">
        <LightbulbIcon aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
        <span>{failure.suggested_fix}</span>
      </p>
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <ArrowsClockwiseIcon aria-hidden="true" className="size-3.5" />
          {failure.retryable
            ? 'Retrying may succeed.'
            : 'Retrying will not help until this is fixed.'}
        </span>
        {failure.docs_url ? (
          <a
            href={failure.docs_url}
            target="_blank"
            rel="noreferrer"
            className="underline underline-offset-2 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
          >
            Read the docs
          </a>
        ) : null}
      </p>
      {failure.log_excerpt ? (
        <div>
          <button
            type="button"
            aria-expanded={open}
            aria-controls={excerptId}
            onClick={() => setOpen((v) => !v)}
            className="flex items-center gap-1 rounded text-xs font-medium text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2"
          >
            <CaretRightIcon
              aria-hidden="true"
              className={`size-3 transition-transform motion-reduce:transition-none ${open ? 'rotate-90' : ''}`}
            />
            Log excerpt
          </button>
          <pre
            id={excerptId}
            hidden={!open}
            tabIndex={0}
            aria-label="Log excerpt"
            className="mt-1 max-h-64 overflow-auto rounded bg-muted p-2 font-mono text-xs whitespace-pre-wrap break-words focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
          >
            {failure.log_excerpt}
          </pre>
        </div>
      ) : null}
    </section>
  )
}
