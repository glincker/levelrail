import type { badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import type { ReconcileCondition } from '../types/deploy'
import type { AppStatusSummary } from '../types/appDetail'

// Shared between routes/apps/$name.tsx (page header) and
// AppScopedSidebar.tsx (sidebar app-info block): both render the same
// one-line status rollup from the same conditions array, so the logic
// lives here once instead of being copy-pasted at both call sites.
// Mirrors how Coolify/Dokploy show a single status pill next to the
// resource name.
export function summarizeAppStatus(conditions: ReconcileCondition[]): {
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

// Solid-fill counterpart to badgeVariants' success/destructive/muted
// backgrounds (components/ui/badge.tsx): a dot has no room for the
// badge's own light background + dark text combo, so this maps the same
// three variants to a single solid color instead. Shared between
// AppRow's StatusDot and DashboardOverview's decorative row dot.
export const STATUS_DOT_COLOR: Record<AppStatusSummary['variant'], string> = {
  success: 'bg-green-500 dark:bg-green-400',
  destructive: 'bg-destructive',
  muted: 'bg-muted-foreground/40',
}
