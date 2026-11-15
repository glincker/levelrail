import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  LoaderCircleGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Fragment } from 'react'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import { formatDurationMs } from '../../lib/deployDuration'
import {
  cacheChipState,
  stageDurationMs,
  type BuildStage,
  type BuildStageName,
  type BuildStageStatus,
} from '../../lib/buildStages'
import { CacheChip } from './CacheChip'
import '@/styles/runMotion.css'

const ICON: Record<BuildStageStatus, UiIcon> = {
  running: LoaderCircleGlassIcon,
  done: CircleCheckGlassIcon,
  failed: CircleAlertGlassIcon,
}

const ICON_CLASS: Record<BuildStageStatus, string> = {
  running: 'text-primary animate-spin motion-reduce:animate-none',
  done: 'text-tone-success',
  failed: 'text-tone-danger',
}

const NODE_CLASS: Record<BuildStageStatus, string> = {
  running: 'border-primary/60 bg-primary/5',
  done: 'border-border bg-card',
  failed: 'border-tone-danger-border bg-tone-danger-soft',
}

/** Horizontal strip of build stages, one button per stage. */
export function StageGraph({
  stages,
  nowMs,
  selected,
  onSelect,
}: {
  stages: BuildStage[]
  nowMs: number
  selected: BuildStageName | null
  onSelect: (stage: BuildStageName) => void
}) {
  const { t } = useDeployDetailT()
  return (
    <ol
      aria-label={t('buildStages.graphLabel')}
      className="flex items-stretch overflow-x-auto pb-1"
    >
      {stages.map((stage, i) => {
        const Icon = ICON[stage.status]
        const duration = formatDurationMs(stageDurationMs(stage, nowMs)) ?? '-'
        const name = t(`buildStages.stage.${stage.stage}`)
        return (
          <Fragment key={stage.stage}>
            {i > 0 ? (
              <li
                aria-hidden="true"
                className="flex shrink-0 items-center"
                role="presentation"
              >
                <span className="h-px w-4 bg-border sm:w-6" />
              </li>
            ) : null}
            <li className="run-enter shrink-0">
              <button
                type="button"
                aria-pressed={selected === stage.stage}
                aria-label={t('buildStages.nodeLabel', {
                  stage: name,
                  status: t(`buildStages.status.${stage.status}`),
                  duration,
                })}
                data-status={stage.status}
                onClick={() => {
                  onSelect(stage.stage)
                }}
                className={`relative flex h-full min-w-28 overflow-hidden flex-col gap-1 rounded-lg border px-3 py-2 text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none ${NODE_CLASS[stage.status]} ${selected === stage.stage ? 'ring-2 ring-ring' : ''}`}
              >
                <span className="flex items-center gap-1.5 text-sm font-medium text-foreground">
                  <Icon
                    className={`size-4 shrink-0 ${ICON_CLASS[stage.status]}`}
                    aria-hidden="true"
                  />
                  {name}
                </span>
                <span className="font-mono text-xs text-muted-foreground">
                  {duration}
                </span>
                <CacheChip state={cacheChipState(stage)} />
                {stage.status === 'running' ? (
                  <span
                    aria-hidden="true"
                    className="absolute inset-x-0 bottom-0 h-0.5 overflow-hidden bg-primary/20"
                  >
                    <span className="run-shimmer block h-full w-1/2 bg-primary" />
                  </span>
                ) : null}
              </button>
            </li>
          </Fragment>
        )
      })}
    </ol>
  )
}
