import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import { formatDurationMs } from '../../lib/deployDuration'
import {
  BUILD_STAGE_ORDER,
  type BuildStagesResponse,
} from '../../lib/buildStages'
import '@/styles/runMotion.css'

const SEGMENT = {
  done: 'bg-tone-success',
  failed: 'bg-tone-danger',
  running: 'bg-primary/30',
  pending: 'bg-muted',
} as const

/** One segment per stage plus elapsed time; no estimate without history. */
export function RunProgress({
  data,
  elapsedMs,
}: {
  data: BuildStagesResponse
  elapsedMs: number
}) {
  const { t } = useDeployDetailT()
  return (
    <div className="space-y-1">
      <ol aria-label={t('run.progressLabel')} className="flex gap-1">
        {BUILD_STAGE_ORDER.map((name) => {
          const stage = data.stages.find((s) => s.stage === name)
          const status = stage?.status ?? 'pending'
          return (
            <li
              key={name}
              data-status={status}
              title={t(`buildStages.stage.${name}`)}
              className={`relative h-1.5 flex-1 overflow-hidden rounded-full ${SEGMENT[status]}`}
            >
              {status === 'running' ? (
                <span
                  aria-hidden="true"
                  className="run-shimmer absolute inset-y-0 w-1/2 bg-primary"
                />
              ) : null}
            </li>
          )
        })}
      </ol>
      <p className="text-xs text-muted-foreground" data-testid="run-elapsed">
        {t('run.elapsed', { time: formatDurationMs(elapsedMs) ?? '-' })}
      </p>
    </div>
  )
}
