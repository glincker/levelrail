import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'
import { ROLLBACK_SECTION_ID } from '../../lib/updatesAnchors'
import type { SelfUpgradeAttempt } from '../../queries/selfUpgrade'
import { UpgradePipeline } from './updates/UpgradePipeline'

interface Props {
  target: string
  plannedSteps: string[]
  attempt: SelfUpgradeAttempt | undefined
  startedAt: number | null
  reconnecting: boolean
  releaseUrl: string | null | undefined
  onDismiss?: () => void
  onRetry?: () => void
}

// UpgradeProgress is the live run view: a CI-style pipeline over the
// persisted attempt, so a reload or a control plane restart resumes it.
export function UpgradeProgress({
  target,
  plannedSteps,
  attempt,
  startedAt,
  reconnecting,
  releaseUrl,
  onDismiss,
  onRetry,
}: Props) {
  const { t } = useTranslation('updates')
  const done = attempt !== undefined && attempt.outcome !== 'running'
  return (
    <div className="space-y-2">
      <UpgradePipeline
        target={target}
        plannedSteps={plannedSteps}
        attempt={attempt}
        startedAt={startedAt}
        reconnecting={reconnecting}
      />
      {done && attempt?.outcome !== 'succeeded' ? (
        <div className="flex flex-wrap gap-2">
          {onRetry ? (
            <Button type="button" size="sm" onClick={onRetry}>
              {t('progress.retry')}
            </Button>
          ) : null}
          {attempt?.outcome === 'failed' ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() =>
                document
                  .getElementById(ROLLBACK_SECTION_ID)
                  ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
              }
            >
              {t('progress.rollBack')}
            </Button>
          ) : null}
        </div>
      ) : null}
      <div className="flex flex-wrap gap-3 text-xs">
        {attempt?.outcome === 'succeeded' && releaseUrl ? (
          <a
            className="text-primary underline underline-offset-2"
            href={releaseUrl}
            target="_blank"
            rel="noreferrer"
          >
            {t('progress.releaseNotes')}
          </a>
        ) : null}
        {done && onDismiss ? (
          <button
            type="button"
            className="text-muted-foreground underline underline-offset-2"
            onClick={onDismiss}
          >
            {t('progress.dismiss')}
          </button>
        ) : null}
      </div>
    </div>
  )
}
