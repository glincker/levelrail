import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { DeployAttempt } from '../types/deployAttempt'
import { formatDeployDuration } from '../lib/deployDuration'
import {
  DEPLOY_ATTEMPT_STATUS_LABEL,
  DEPLOY_ATTEMPT_STATUS_TONE,
} from '../lib/deployAttemptPresentation'
import { commitSubject } from '../lib/deploymentPresentation'
import { cn } from '@/lib/utils'

const TONE_TEXT: Record<string, string> = {
  success: 'text-tone-success',
  danger: 'text-tone-danger',
  warning: 'text-tone-warning',
  info: 'text-tone-info',
  neutral: 'text-muted-foreground',
}

// The pinned latest attempt's headline: status, stage, duration, and for a
// failure a direct link to the log (the detail page opens on the failing
// stage's tab by itself).
export function DeployPinnedStatus({
  appName,
  attempt,
  stageLabel,
}: {
  appName: string
  attempt: DeployAttempt
  stageLabel?: string
}) {
  const { t } = useTranslation('common')
  const tone = DEPLOY_ATTEMPT_STATUS_TONE[attempt.status]
  const parts = [
    DEPLOY_ATTEMPT_STATUS_LABEL[attempt.status],
    stageLabel,
    formatDeployDuration(attempt.started_at, attempt.finished_at),
  ].filter((p): p is string => Boolean(p))
  return (
    <p
      data-testid="deploy-pinned-status"
      className="mt-1 flex flex-wrap items-center gap-x-2 text-sm"
    >
      <span className={cn('font-medium', TONE_TEXT[tone])}>
        {parts.join(' · ')}
      </span>
      {attempt.status === 'failed' ? (
        <Link
          to="/apps/$name/deploys/$deployId/logs"
          params={{ name: appName, deployId: attempt.id }}
          aria-label={t('deployHistory.failedLogLabel', {
            title:
              (attempt.commit_message &&
                commitSubject(attempt.commit_message)) ||
              attempt.image,
          })}
          className="relative z-10 text-sm font-medium text-foreground underline underline-offset-2"
        >
          {t('deployHistory.failedLog')}
        </Link>
      ) : null}
    </p>
  )
}
