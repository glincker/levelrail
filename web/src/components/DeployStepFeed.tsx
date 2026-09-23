import {
  CheckCircleIcon,
  CircleIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type {
  DeployStepEvent,
  DeployStepStatus,
} from '../hooks/useDeployStepStream'
import { cn } from '@/lib/utils'

// The pipeline's own fixed phase order (internal/api/builds.go's step
// emission): a step this list doesn't yet have an event for renders as
// "pending" rather than being omitted, so the list itself never
// reflows as events arrive, only each row's icon/label does.
const KNOWN_STEPS: { key: string; label: string }[] = [
  { key: 'detecting', label: 'Detecting framework' },
  { key: 'building', label: 'Building' },
  { key: 'pushing', label: 'Loading image' },
  { key: 'deploying', label: 'Deploying' },
]

type RowStatus = DeployStepStatus | 'pending'

function statusFor(steps: DeployStepEvent[], key: string): RowStatus {
  return steps.find((s) => s.step === key)?.status ?? 'pending'
}

function StatusIcon({ status }: { status: RowStatus }) {
  switch (status) {
    case 'done':
      return (
        <CheckCircleIcon
          className="size-4 text-success-foreground"
          weight="fill"
          aria-hidden="true"
        />
      )
    case 'running':
      return (
        <SpinnerGapIcon
          className="size-4 animate-spin text-muted-foreground"
          aria-hidden="true"
        />
      )
    case 'failed':
      return (
        <WarningCircleIcon
          className="size-4 text-destructive"
          weight="fill"
          aria-hidden="true"
        />
      )
    default:
      return (
        <CircleIcon
          className="size-4 text-muted-foreground/50"
          aria-hidden="true"
        />
      )
  }
}

// DeployStepFeed renders the build-trigger flow's named pipeline steps
// as a checklist rather than a scrolling log: GET
// .../deploys/{deployId}/steps's own contract (internal/api/
// deploy_steps.go) alongside detecting/building/pushing/deploying is a
// terminal "failed" event that replaces the normal phase order, since a
// failure can land on any of the four steps.
export function DeployStepFeed({ steps }: { steps: DeployStepEvent[] }) {
  const failed = steps.find((s) => s.step === 'failed')

  return (
    <ol className="flex flex-col gap-1.5">
      {KNOWN_STEPS.map(({ key, label }) => {
        // Each row shows its own last known status as-is: a step that
        // never got an event (because an earlier one already failed)
        // correctly stays "pending" rather than being inferred as done.
        const status = statusFor(steps, key)
        return (
          <li key={key} className="flex items-center gap-2 text-sm">
            <StatusIcon status={status} />
            <span
              className={cn(
                status === 'pending' && 'text-muted-foreground',
                status === 'failed' && 'text-destructive',
              )}
            >
              {label}
            </span>
          </li>
        )
      })}
      {failed ? (
        <li className="flex items-center gap-2 text-sm text-destructive">
          <StatusIcon status="failed" />
          <span>Failed</span>
        </li>
      ) : null}
    </ol>
  )
}
