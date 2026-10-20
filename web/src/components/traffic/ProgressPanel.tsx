import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { StepList, type Step } from './StepList'
import { useAgeLabel } from './useAgeLabel'

export type RunState = 'running' | 'succeeded' | 'failed'

export interface GoLiveRun {
  id: string
  state: RunState
  steps: readonly Step[]
  startedAt: string
  updatedAt?: string
  undoable?: boolean
}

export interface ProgressPanelProps {
  domain: string
  run: GoLiveRun
  siteUrl?: string | null
  /** Replaces the default undo button, e.g. a trigger for ConfirmChangesDialog. */
  undoSlot?: ReactNode
  onUndo?: () => void
  onRetryStep?: (stepId: string) => void
  onOpenDoctor?: () => void
  onDismiss?: () => void
  className?: string
}

/** Go live view for one domain: steps, elapsed time and the way back out. */
export function ProgressPanel({
  domain,
  run,
  siteUrl,
  undoSlot,
  onUndo,
  onRetryStep,
  onOpenDoctor,
  onDismiss,
  className,
}: Readonly<ProgressPanelProps>) {
  const { t } = useTranslation('traffic')
  const titleId = useId()
  const running = run.state === 'running'
  const elapsed = useAgeLabel(running ? run.startedAt : null)
  const updated = useAgeLabel(running ? run.updatedAt : null)

  const steps = run.steps.map((step) =>
    step.state === 'failed' && onRetryStep && !step.action
      ? {
          ...step,
          action: (
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => onRetryStep(step.id)}
            >
              {t('progress.retry')}
            </Button>
          ),
        }
      : step,
  )

  const title =
    run.state === 'succeeded'
      ? t('progress.done', { domain })
      : run.state === 'failed'
        ? t('progress.failed', { domain })
        : t('progress.title', { domain })

  const undo =
    undoSlot ??
    (run.undoable && onUndo ? (
      <Button type="button" variant="outline" size="sm" onClick={onUndo}>
        {t('progress.undo')}
      </Button>
    ) : null)

  return (
    <section
      aria-labelledby={titleId}
      aria-busy={running}
      className={cn(
        'space-y-4 rounded-lg border border-border bg-card p-4',
        className,
      )}
    >
      <header className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 id={titleId} className="text-sm font-semibold">
          {title}
        </h3>
        {running ? (
          <p className="text-xs text-muted-foreground">
            {elapsed ? t('progress.elapsed', { age: elapsed }) : null}
            {elapsed && updated ? ' · ' : null}
            {updated ? t('progress.updated', { age: updated }) : null}
          </p>
        ) : null}
      </header>
      <StepList steps={steps} />
      <footer className="flex flex-wrap items-center gap-2">
        {run.state === 'succeeded' && siteUrl ? (
          <a
            href={siteUrl}
            target="_blank"
            rel="noreferrer"
            className={buttonVariants({ size: 'sm' })}
          >
            {t('progress.openSite')}
          </a>
        ) : null}
        {run.state === 'failed' && onOpenDoctor ? (
          <Button type="button" size="sm" onClick={onOpenDoctor}>
            {t('progress.openDoctor')}
          </Button>
        ) : null}
        {undo}
        {!running && onDismiss ? (
          <Button type="button" variant="ghost" size="sm" onClick={onDismiss}>
            {t('progress.dismiss')}
          </Button>
        ) : null}
      </footer>
    </section>
  )
}
