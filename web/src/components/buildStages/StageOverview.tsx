import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import {
  BUILD_STAGE_ORDER,
  failedStage,
  runElapsedMs,
  type BuildStageName,
} from '../../lib/buildStages'
import { RunProgress } from './RunProgress'
import { StageGraph } from './StageGraph'
import { useBuildStages } from './useBuildStages'

/** Stage graph with run progress, loading, error and empty states. */
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
  const { query, data, nowMs } = useBuildStages(appName, deployId)

  if (query.isPending) {
    return (
      <div
        aria-busy="true"
        aria-label={t('buildStages.loading')}
        className="flex gap-2 overflow-x-auto"
      >
        {BUILD_STAGE_ORDER.map((s) => (
          <Skeleton key={s} className="h-16 w-28 shrink-0" />
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
  return (
    <div className="space-y-3">
      <RunProgress data={data} elapsedMs={runElapsedMs(data, nowMs)} />
      <StageGraph
        stages={data.stages}
        nowMs={nowMs}
        selected={selected}
        onSelect={onSelect}
      />
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
