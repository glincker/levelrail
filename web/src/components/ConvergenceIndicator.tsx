import {
  CheckCircleIcon,
  WarningCircleIcon,
  SpinnerGapIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import type { ReconcileCondition } from '../types/deploy'
import { deriveConvergence, type ConvergenceState } from '../lib/convergence'
import { Badge, type badgeVariants } from '@/components/ui/badge'

const VARIANT: Record<
  Exclude<ConvergenceState, 'unknown'>,
  VariantProps<typeof badgeVariants>['variant']
> = {
  converged: 'success',
  reconciling: 'muted',
  error: 'destructive',
}

const LABEL: Record<Exclude<ConvergenceState, 'unknown'>, string> = {
  converged: 'Converged',
  reconciling: 'Reconciling...',
  error: 'Error',
}

const ICON: Record<Exclude<ConvergenceState, 'unknown'>, Icon> = {
  converged: CheckCircleIcon,
  reconciling: SpinnerGapIcon,
  error: WarningCircleIcon,
}

// Second, small badge next to the app header's summarizeAppStatus one:
// that badge summarizes overall health, this one answers "does the
// running state match what the reconciler last reported," from the same
// conditions array (routes/apps/$name.tsx already fetches it via
// useDeployStatus, no separate query here). Renders nothing for
// 'unknown' (no conditions yet), since summarizeAppStatus's own "No
// status yet" badge already covers that case.
export function ConvergenceIndicator({
  conditions,
}: {
  conditions: ReconcileCondition[]
}) {
  const state = deriveConvergence(conditions)
  if (state === 'unknown') {
    return null
  }
  const ConvergenceIcon = ICON[state]

  return (
    <Badge variant={VARIANT[state]} className="shrink-0">
      <ConvergenceIcon
        className={state === 'reconciling' ? 'size-3 animate-spin' : 'size-3'}
      />
      {LABEL[state]}
    </Badge>
  )
}
