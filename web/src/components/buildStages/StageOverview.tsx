import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useTranslation } from 'react-i18next'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import {
  BUILD_STAGE_ORDER,
  failedStage,
  firstCacheMiss,
  type BuildStageName,
} from '../../lib/buildStages'
import { StageWaterfall } from './StageWaterfall'
import { useBuildStages } from './useBuildStages'

/** The one stage waterfall with loading, error and empty states. */
export function StageOverview({
  appName,
  deployId,
  selected,
  onSelect,
}: {
  appName: string
  deployId: string
  selected: BuildStageName | null
  onSelect: (stage: BuildStageName) => void
}) {
  const { t } = useDeployDetailT()
  const { t: tm } = useTranslation('buildLogs', { keyPrefix: 'cacheMiss' })
  const { query, data, nowMs } = useBuildStages(appName, deployId)

  if (query.isPending) {
    return (
      <div
        aria-busy="true"
        aria-label={t('buildStages.loading')}
        className="space-y-1"
      >
        {BUILD_STAGE_ORDER.map((s) => (
          <Skeleton key={s} className="h-6 w-full" />
        ))}
      </div>
    )
  }
  if (query.isError || !data) {
    return (
      <Banner
        variant="critical"
        title={t('buildStages.error')}
        action={
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
          >
            <RefreshCwGlassIcon className="size-4" aria-hidden="true" />
            {t('buildStages.retry')}
          </Button>
        }
      />
    )
  }
  if (data.stages.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('buildStages.empty')}</p>
    )
  }
  const failed = failedStage(data)
  const miss = firstCacheMiss(data)
  return (
    <div className="space-y-2">
      <StageWaterfall
        data={data}
        nowMs={nowMs}
        selected={selected}
        onSelect={onSelect}
      />
      {miss ? (
        <div data-testid="cache-miss-hint">
          <Banner variant="info" title={tm('title')}>
            {tm('body', {
              stage: t(`buildStages.stage.${miss.stage}`),
              step: miss.step,
              cause: tm(`cause.${miss.cause}`, {
                defaultValue: tm('cause.step_changed'),
              }),
            })}
          </Banner>
        </div>
      ) : null}
      {failed ? (
        <div data-testid="stage-failure-hint">
          <Banner
            variant="critical"
            title={t('buildStages.failedHint', {
              stage: t(`buildStages.stage.${failed.stage}`),
            })}
          />
        </div>
      ) : null}
    </div>
  )
}
