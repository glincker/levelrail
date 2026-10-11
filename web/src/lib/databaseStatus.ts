import type { badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import type { ReconcileCondition } from '../types/deploy'
import { isOptionalFeatureUnconfigured } from './appStatus'

// Database counterpart to summarizeAppStatus, kept in step with the
// server-side rollup that feeds the list endpoint's status field.
export function summarizeDatabaseStatus(conditions: ReconcileCondition[]): {
  label: string
  variant: VariantProps<typeof badgeVariants>['variant']
} {
  if (conditions.length === 0) {
    return { label: 'No status yet', variant: 'muted' }
  }
  if (conditions.some((c) => c.Reason === 'Suspended')) {
    return { label: 'Stopped', variant: 'muted' }
  }
  if (conditions.some((c) => c.Status === 'False')) {
    return { label: 'Attention needed', variant: 'destructive' }
  }
  if (
    conditions.every(
      (c) => c.Status === 'True' || isOptionalFeatureUnconfigured(c),
    )
  ) {
    return { label: 'Healthy', variant: 'success' }
  }
  return { label: 'Reconciling', variant: 'muted' }
}
