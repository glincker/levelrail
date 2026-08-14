import type { badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import type { ReconcileCondition } from '../types/deploy'

// Database counterpart to lib/appStatus.ts's summarizeAppStatus, same
// reasoning: shared between routes/databases/$name.tsx (page header) and
// DatabaseScopedSidebar.tsx (sidebar database-info block), both render
// the same one-line status rollup from the same conditions array, so the
// logic lives here once instead of being copy-pasted at both call sites.
export function summarizeDatabaseStatus(conditions: ReconcileCondition[]): {
  label: string
  variant: VariantProps<typeof badgeVariants>['variant']
} {
  if (conditions.length === 0) {
    return { label: 'No status yet', variant: 'muted' }
  }
  if (conditions.some((c) => c.Status === 'False')) {
    return { label: 'Attention needed', variant: 'destructive' }
  }
  if (conditions.every((c) => c.Status === 'True')) {
    return { label: 'Healthy', variant: 'success' }
  }
  return { label: 'Reconciling', variant: 'muted' }
}
