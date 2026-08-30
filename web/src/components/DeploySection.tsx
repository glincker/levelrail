import { useState, type ReactNode } from 'react'
import {
  CaretDownIcon,
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
import { Card } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsiblePanel,
} from '@/components/ui/collapsible'

// Mirrors DeployStageTimeline's own status vocabulary (icon/color/label):
// duplicated rather than imported since that component renders a compact
// multi-stage list and this one renders a single stage as its own
// expandable card, different enough layouts that sharing more than the
// vocabulary would couple them for no real benefit.
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

// One real pipeline stage (Build or Roll out, see lib/deployStages.ts) as
// its own collapsible card: status icon and a one-line summary stay
// visible collapsed, expanding reveals stage-specific real detail
// (children), the build log tail or the reconcile conditions.
export function DeploySection({
  stage,
  summary,
  defaultOpen,
  children,
}: {
  stage: DeployStage
  summary: string
  defaultOpen: boolean
  children: ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)
  const StatusIcon = STATUS_ICON[stage.status]
  const elapsed = useSectionElapsed(stage)

  return (
    <Card size="sm" className="py-0">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="group/deploy-section flex items-start gap-3 px-4 py-3">
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
                <span className="text-xs text-muted-foreground/70">
                  {elapsed}
                </span>
              ) : null}
            </div>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {summary}
            </p>
          </div>
          <CaretDownIcon
            className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-data-panel-open/deploy-section:rotate-180"
            aria-hidden="true"
          />
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <div className="border-t border-border px-4 py-3">{children}</div>
        </CollapsiblePanel>
      </Collapsible>
    </Card>
  )
}

function useSectionElapsed(stage: DeployStage): string | null {
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
