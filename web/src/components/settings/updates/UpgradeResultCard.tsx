import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { SelfUpgradeAttempt } from '../../../queries/selfUpgrade'
import { FailureActions } from './FailureActions'

const UpgradePipeline = lazy(() =>
  import('./UpgradePipeline').then((m) => ({ default: m.UpgradePipeline })),
)

interface Props {
  attempt: SelfUpgradeAttempt
  verified: boolean
  releaseUrl: string | null | undefined
  onDismiss: () => void
}

// UpgradeResultCard is the compact, persistent outcome of the last finished
// upgrade; the stage timeline opens on demand.
export function UpgradeResultCard({
  attempt,
  verified,
  releaseUrl,
  onDismiss,
}: Props) {
  const { t } = useTranslation('updates')
  const [open, setOpen] = useState(false)
  const ok = attempt.outcome === 'succeeded'
  const time = new Date(attempt.finished_at || attempt.started_at)
  const params = {
    from: attempt.from_version,
    to: attempt.to_version,
    time: Number.isNaN(time.getTime()) ? '' : time.toLocaleString(),
    reason: attempt.error || attempt.failed_step,
  }
  let line: string
  if (ok)
    line = t(
      verified ? 'result.succeeded' : 'result.succeededUnverified',
      params,
    )
  else if (attempt.outcome === 'rolled_back')
    line = t('result.rolledBack', params)
  else line = t('result.failed', params)

  return (
    <section
      aria-label={line}
      className={cn(
        'space-y-3 rounded-md border p-3 text-sm',
        ok
          ? 'border-success/30 bg-success/10'
          : 'border-destructive/30 bg-destructive/5',
      )}
    >
      <div className="flex items-start gap-2">
        {ok ? (
          <CircleCheckGlassIcon
            className="mt-0.5 size-5 shrink-0 text-success"
            aria-hidden
          />
        ) : (
          <CircleAlertGlassIcon
            className="mt-0.5 size-5 shrink-0 text-destructive"
            aria-hidden
          />
        )}
        <p className="min-w-0 flex-1" role="status">
          {line}
        </p>
      </div>
      {!ok ? <FailureActions attempt={attempt} /> : null}
      <div className="flex flex-wrap gap-3 text-xs">
        {ok && releaseUrl ? (
          <a
            className="text-primary underline underline-offset-2"
            href={releaseUrl}
            target="_blank"
            rel="noreferrer"
          >
            {t('result.notes')}
          </a>
        ) : null}
        <button
          type="button"
          aria-expanded={open}
          className="text-primary underline underline-offset-2"
          onClick={() => setOpen((v) => !v)}
        >
          {open ? t('result.hideDetails') : t('result.details')}
        </button>
        <button
          type="button"
          className="text-muted-foreground underline underline-offset-2"
          onClick={onDismiss}
        >
          {t('result.dismiss')}
        </button>
      </div>
      {open ? (
        <Suspense fallback={null}>
          <UpgradePipeline
            target={attempt.to_version}
            plannedSteps={[]}
            attempt={attempt}
            startedAt={null}
            reconnecting={false}
            showReload={false}
          />
        </Suspense>
      ) : null}
    </section>
  )
}
