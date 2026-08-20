import {
  CheckCircleIcon,
  CircleIcon,
  MinusCircleIcon,
  QuestionIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { DeployStage, DeployStageStatus } from '../lib/deployStages'
import { formatDurationMs } from '../lib/deployDuration'
import { useNowTick } from '../hooks/useNowTick'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// The at-a-glance primary element for the live deploy view (Build, then
// Roll out), the Jenkins/GitHub-Actions-style stage list the app
// overview's "Deploy in progress" banner links into. See
// lib/deployStages.ts for how each stage's status is derived; this
// component only renders it.
export function DeployStageTimeline({ stages }: { stages: DeployStage[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm text-muted-foreground uppercase tracking-wide">
          Deploy stages
        </CardTitle>
      </CardHeader>
      <CardContent>
        <ol className="space-y-4">
          {stages.map((stage) => (
            <DeployStageRow key={stage.key} stage={stage} />
          ))}
        </ol>
      </CardContent>
    </Card>
  )
}

const STATUS_ICON: Record<DeployStageStatus, Icon> = {
  pending: CircleIcon,
  running: SpinnerGapIcon,
  done: CheckCircleIcon,
  failed: WarningCircleIcon,
  skipped: MinusCircleIcon,
  unknown: QuestionIcon,
}

const STATUS_ICON_CLASS: Record<DeployStageStatus, string> = {
  pending: 'text-muted-foreground',
  running: 'text-primary animate-spin',
  done: 'text-green-600 dark:text-green-400',
  failed: 'text-destructive',
  skipped: 'text-muted-foreground/60',
  unknown: 'text-muted-foreground/60',
}

const STATUS_LABEL: Record<DeployStageStatus, string> = {
  pending: 'Pending',
  running: 'Running',
  done: 'Done',
  failed: 'Failed',
  skipped: 'Skipped',
  unknown: 'Unknown',
}

function DeployStageRow({ stage }: { stage: DeployStage }) {
  const StatusIcon = STATUS_ICON[stage.status]
  const elapsed = useStageElapsed(stage)

  return (
    <li className="flex items-start gap-3">
      <StatusIcon
        className={`mt-0.5 size-4 shrink-0 ${STATUS_ICON_CLASS[stage.status]}`}
        aria-hidden="true"
      />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="text-sm font-medium text-foreground">
            {stage.label}
          </span>
          <span className="text-xs text-muted-foreground">
            {STATUS_LABEL[stage.status]}
          </span>
          {elapsed ? (
            <span className="text-xs text-muted-foreground/70">{elapsed}</span>
          ) : null}
        </div>
        {stage.detail ? (
          <p
            className={`mt-0.5 text-xs ${
              stage.status === 'failed'
                ? 'text-destructive'
                : 'text-muted-foreground'
            }`}
          >
            {stage.detail}
          </p>
        ) : null}
      </div>
    </li>
  )
}

// useStageElapsed live-ticks a running stage's elapsed time once a second,
// or formats a finished stage's fixed duration once. Pending, skipped,
// and unknown stages have no meaningful duration to show.
function useStageElapsed(stage: DeployStage): string | null {
  const now = useNowTick(stage.status === 'running' && !!stage.startedAt)

  if (stage.status === 'running' && stage.startedAt) {
    return formatDurationMs(now - new Date(stage.startedAt).getTime())
  }
  if (
    (stage.status === 'done' || stage.status === 'failed') &&
    stage.startedAt &&
    stage.finishedAt
  ) {
    return formatDurationMs(
      new Date(stage.finishedAt).getTime() -
        new Date(stage.startedAt).getTime(),
    )
  }
  return null
}
