import { CopyGlassIcon, PauseGlassIcon, PlayGlassIcon } from '@thesvg/ui-react'
import { useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { LogTerminal } from '../LogTerminal'
import { useLogStream } from '../../hooks/useLogStream'
import { buildStageLogUrl } from '../../queries/buildStages'
import { logLinesToText } from '../../lib/logFilter'
import type { BuildStageName } from '../../lib/buildStages'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'

/** Live or stored tail of one stage, over the stage-filtered log SSE. */
export function StageLogPanel({
  appName,
  deployId,
  stage,
  finished,
}: {
  appName: string
  deployId: string
  stage: BuildStageName
  finished: boolean
}) {
  const { t } = useDeployDetailT()
  const url = buildStageLogUrl(appName, deployId, stage)
  const { lines, isPaused, pause, resume, connectionState } = useLogStream(url)
  const copy = useCallback(() => {
    void navigator.clipboard.writeText(logLinesToText(lines))
  }, [lines])
  const stageName = t(`buildStages.stage.${stage}`)
  return (
    <section
      aria-label={t('buildStages.logLabel', { stage: stageName })}
      className="space-y-2"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium text-foreground">
          {t('buildStages.logTitle', { stage: stageName })}
        </h3>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-pressed={!isPaused}
            onClick={isPaused ? resume : pause}
          >
            {isPaused ? (
              <PlayGlassIcon className="size-4" aria-hidden="true" />
            ) : (
              <PauseGlassIcon className="size-4" aria-hidden="true" />
            )}
            {t(isPaused ? 'buildStages.followOff' : 'buildStages.followOn')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={lines.length === 0}
            onClick={copy}
          >
            <CopyGlassIcon className="size-4" aria-hidden="true" />
            {t('buildStages.copy')}
          </Button>
        </div>
      </div>
      {connectionState === 'error' ? (
        <p role="alert" className="text-xs text-destructive">
          {t('buildStages.logError')}
        </p>
      ) : null}
      <LogTerminal
        lines={lines}
        isPaused={isPaused}
        pause={pause}
        resume={resume}
        heightClassName="h-64"
        emptyStateMessage={t('buildStages.logEmpty')}
        isFinished={finished}
      />
    </section>
  )
}
