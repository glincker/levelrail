import type { DeployStage, DeployStageStatus } from '../../lib/deployStages'
import type { DeployState } from '../../lib/deployState'

export interface TimelineRow {
  key: string
  stage: DeployStage
  /** i18n key for the reason line when the stage has no detail of its own. */
  reasonKey?: string
}

/** buildTimelineRows maps stages to rows; static sites get Build then Publish. */
export function buildTimelineRows(
  build: DeployStage,
  subStages: DeployStage[],
  state: DeployState,
  finishedAt?: string,
): TimelineRow[] {
  const buildRow: TimelineRow = {
    key: 'build',
    stage: build,
    reasonKey:
      build.status === 'skipped'
        ? 'rollout.reason.skippedBuild'
        : build.status === 'done'
          ? 'rollout.reason.doneBuild'
          : undefined,
  }
  if (state.isStatic) {
    const status: DeployStageStatus =
      state.phase === 'rollout_failed'
        ? 'failed'
        : state.phase === 'live'
          ? 'done'
          : 'pending'
    return [
      buildRow,
      {
        key: 'publish',
        stage: {
          key: 'rollout',
          label: 'publish',
          status,
          startedAt: finishedAt,
          finishedAt: status === 'done' ? finishedAt : undefined,
        },
        reasonKey: status === 'done' ? 'rollout.reason.donePublish' : undefined,
      },
    ]
  }
  const noImage = state.waitingReason === 'AwaitingFirstBuild'
  return [
    buildRow,
    ...subStages.map((s): TimelineRow => {
      let reasonKey: string | undefined
      if (s.key === 'health-check') {
        if (s.status === 'running') {
          reasonKey = noImage
            ? 'rollout.reason.runningNoImage'
            : 'rollout.reason.runningHealth'
        } else if (s.status === 'pending') {
          reasonKey = 'rollout.reason.pendingHealth'
        }
      } else if (s.key === 'cutover' && s.status === 'pending') {
        reasonKey = 'rollout.reason.pendingCutover'
      } else if (s.key === 'cleanup' && s.status === 'pending') {
        reasonKey = 'rollout.reason.pendingCleanup'
      }
      return { key: s.key, stage: s, reasonKey }
    }),
  ]
}
