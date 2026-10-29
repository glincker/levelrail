import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LightbulbGlassIcon } from '@thesvg/ui-react'
import { useAttentionSummary } from '../shell/useAttentionSummary'
import { SetupChip } from './SetupChip'

export function RecommendationsLineView({
  count,
  showSetup,
}: {
  count: number
  showSetup: boolean
}) {
  const { t } = useTranslation('dashboard')
  if (count === 0 && !showSetup) return null
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-muted-foreground">
      {count > 0 ? (
        <Link
          to="/status"
          className="inline-flex items-center gap-1.5 hover:text-foreground"
        >
          <LightbulbGlassIcon aria-hidden="true" className="size-4.5" />
          {t('recommendations.line', { count })}
        </Link>
      ) : null}
      {showSetup ? <SetupChip /> : null}
    </div>
  )
}

// One quiet line: the advice itself lives in the notification centre.
export function RecommendationsLine({ showSetup }: { showSetup: boolean }) {
  const { recommendations } = useAttentionSummary()
  return (
    <RecommendationsLineView
      count={recommendations.length}
      showSetup={showSetup}
    />
  )
}
