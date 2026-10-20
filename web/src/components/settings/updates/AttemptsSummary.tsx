import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { summarizeAttempts } from '@/lib/upgradeAttempts'
import { formatRelativeTime } from '@/lib/relativeTime'
import type { SelfUpgradeAttempt } from '@/queries/selfUpgrade'
import { VersionLabel } from './VersionLabel'

export function AttemptsSummary({
  attempts,
  currentVersion,
}: {
  attempts: SelfUpgradeAttempt[]
  currentVersion?: string
}) {
  const { t } = useTranslation('updates')
  const [now] = useState(() => Date.now())
  const { lastOk, failed30 } = summarizeAttempts(attempts, now)
  return (
    <dl className="mb-3 grid gap-3 rounded-md bg-muted/40 px-3 py-2 sm:grid-cols-3">
      <div className="min-w-0">
        <dt className="text-xs text-muted-foreground">
          {t('selfUpgrade.summary.current')}
        </dt>
        <dd className="text-sm">
          {currentVersion ? <VersionLabel value={currentVersion} /> : '-'}
        </dd>
      </div>
      <div>
        <dt className="text-xs text-muted-foreground">
          {t('selfUpgrade.summary.lastOk')}
        </dt>
        <dd className="text-sm">
          {lastOk
            ? formatRelativeTime(lastOk.started_at)
            : t('selfUpgrade.summary.never')}
        </dd>
      </div>
      <div>
        <dt className="text-xs text-muted-foreground">
          {t('selfUpgrade.summary.failed')}
        </dt>
        <dd className="text-sm tabular-nums">{failed30}</dd>
      </div>
    </dl>
  )
}
