import {
  CheckCircleIcon,
  CircleIcon,
  MinusCircleIcon,
  QuestionIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { ConditionsPanel } from '../ConditionsPanel'
import { DeployGatePanel } from '../DeployGatePanel'
import { ProbeAttemptsPanel } from '../ProbeAttemptsPanel'
import { formatDurationMs } from '../../lib/deployDuration'
import type { DeployStageStatus } from '../../lib/deployStages'
import type { DeployState } from '../../lib/deployState'
import type { ReconcileCondition } from '../../types/deploy'
import type { ProbeAttempt } from '../../types/probeAttempt'
import { RolloutCountsPanel } from './RolloutCountsPanel'
import { useDeployDetailT } from './useDeployDetailT'
import type { TimelineRow } from './timelineRows'

const ICON: Record<DeployStageStatus, Icon> = {
  pending: CircleIcon,
  running: SpinnerGapIcon,
  done: CheckCircleIcon,
  failed: WarningCircleIcon,
  skipped: MinusCircleIcon,
  unknown: QuestionIcon,
}

const ICON_CLASS: Record<DeployStageStatus, string> = {
  pending: 'text-muted-foreground',
  running: 'text-info animate-spin motion-reduce:animate-none',
  done: 'text-success',
  failed: 'text-tone-danger',
  skipped: 'text-muted-foreground/60',
  unknown: 'text-muted-foreground/60',
}

function when(iso?: string): string | null {
  if (!iso) return null
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? null : d.toLocaleTimeString()
}

function RolloutRow({
  row,
  state,
  nowMs,
  conditions,
  last,
}: {
  row: TimelineRow
  state: DeployState
  nowMs: number
  conditions: ReconcileCondition[]
  last: boolean
}) {
  const { t } = useDeployDetailT()
  const { stage } = row
  const StageIcon = ICON[stage.status]
  const start = stage.startedAt ? new Date(stage.startedAt).getTime() : null
  const end = stage.finishedAt ? new Date(stage.finishedAt).getTime() : null
  let duration: string | null = null
  if (start !== null && end !== null) {
    duration = formatDurationMs(end - start)
  } else if (start !== null && stage.status === 'running') {
    duration = formatDurationMs(nowMs - start)
  }
  const stuck = state.stuck && stage.status === 'running'
  const reason = stage.detail ?? (row.reasonKey ? t(row.reasonKey) : null)
  const label = t(`rollout.stage.${row.key === 'build' ? 'build' : row.key}`)
  const started = when(stage.startedAt)
  const finished = when(stage.finishedAt)
  const showConditions =
    (stage.status === 'running' || stage.status === 'failed') &&
    row.key === 'health-check' &&
    conditions.length > 0

  return (
    <li className="relative flex gap-3 pb-5 last:pb-0">
      {last ? null : (
        <span
          className="absolute top-6 bottom-0 left-2 w-px bg-border"
          aria-hidden="true"
        />
      )}
      <StageIcon
        className={cn(
          'relative z-10 mt-0.5 size-4 shrink-0 bg-background',
          ICON_CLASS[stage.status],
        )}
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-sm font-medium text-foreground">{label}</span>
          <span className="text-xs text-muted-foreground">
            {t(`rollout.stageStatus.${stage.status}`)}
          </span>
          {duration ? (
            <span className="text-xs text-muted-foreground/80">
              {stage.status === 'running'
                ? t('rollout.runningFor', { duration })
                : t('rollout.duration', { duration })}
            </span>
          ) : null}
          {stuck ? (
            <Badge variant="warning">
              {t('rollout.reason.stuck', {
                limit: formatDurationMs(state.stuckAfterMs ?? 0) ?? '',
              })}
            </Badge>
          ) : null}
        </div>
        {reason ? (
          <p
            className={cn(
              'text-sm',
              stage.status === 'failed'
                ? 'text-tone-danger'
                : 'text-muted-foreground',
            )}
          >
            {reason}
          </p>
        ) : null}
        {started || finished || showConditions ? (
          <details className="text-xs text-muted-foreground">
            <summary className="cursor-pointer">{t('rollout.details')}</summary>
            <div className="space-y-1 pt-1">
              {started ? <p>{t('rollout.started', { at: started })}</p> : null}
              {finished ? (
                <p>{t('rollout.finished', { at: finished })}</p>
              ) : null}
              {showConditions ? (
                <ul className="space-y-0.5 font-mono break-words">
                  {conditions.map((c) => (
                    <li key={`${c.Type}-${c.Reason}`}>
                      {t('rollout.conditionLine', {
                        reason: c.Reason,
                        status: c.Status,
                        message: c.Message,
                      })}
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
          </details>
        ) : null}
      </div>
    </li>
  )
}

/** RolloutTab is a vertical timeline of stages with reasons, durations and stuck hints. */
export function RolloutTab({
  appName,
  deployId,
  rows,
  state,
  nowMs,
  conditions,
  probeAttempts,
}: {
  appName: string
  deployId: string
  rows: TimelineRow[]
  state: DeployState
  nowMs: number
  conditions: ReconcileCondition[]
  probeAttempts?: ProbeAttempt[]
}) {
  const { t } = useDeployDetailT()
  return (
    <div className="space-y-4">
      <RolloutCountsPanel appName={appName} />
      <section
        aria-label={t('rollout.title')}
        className="rounded-lg border border-border p-4"
      >
        <h3 className="mb-3 text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {t('rollout.title')}
        </h3>
        {state.isStatic ? (
          <p className="mb-3 text-sm text-muted-foreground">
            {t('rollout.staticNote')}
          </p>
        ) : null}
        <ol>
          {rows.map((row, i) => (
            <RolloutRow
              key={row.key}
              row={row}
              state={state}
              nowMs={nowMs}
              conditions={conditions}
              last={i === rows.length - 1}
            />
          ))}
        </ol>
      </section>

      <details className="rounded-lg border border-border px-4 py-3">
        <summary className="cursor-pointer text-sm font-medium text-foreground">
          {t('rollout.technical')}
        </summary>
        <div className="space-y-5 pt-3">
          <ConditionsPanel conditions={conditions} />
          {state.isStatic ? null : (
            <>
              <DeployGatePanel appName={appName} deployId={deployId} />
              {probeAttempts && probeAttempts.length > 0 ? (
                <ProbeAttemptsPanel attempts={probeAttempts} />
              ) : null}
            </>
          )}
        </div>
      </details>
    </div>
  )
}
