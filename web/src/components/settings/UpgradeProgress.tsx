import { useTranslation } from 'react-i18next'
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
