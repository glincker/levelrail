import type { badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import type { ReconcileCondition } from '../types/deploy'
import type { AppStatusSummary } from '../types/appDetail'

// Shared between routes/apps/$name.tsx (page header) and
// AppScopedSidebar.tsx (sidebar app-info block): both render the same
// one-line status rollup from the same conditions array, so the logic
// lives here once instead of being copy-pasted at both call sites.
export function summarizeAppStatus(conditions: ReconcileCondition[]): {
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

// An Unknown condition reporting that an optional per-service feature
// (egress policy today) was simply never turned on, the same
// "Disabled"/Unknown idiom the cloudflare-tunnel and registry
// controllers already use for an unconfigured optional integration.
// Without this, a service with no egress policy configured (the common
// case) carries a permanently-Unknown EgressPolicyReady condition that
// never resolves, keeping the app stuck on "Reconciling" forever.
function isOptionalFeatureUnconfigured(c: ReconcileCondition): boolean {
  return (
    c.Status === 'Unknown' &&
    (c.Reason === 'NotConfigured' || c.Reason === 'Disabled')
  )
}

// Solid-fill counterpart to badgeVariants' success/destructive/muted
// backgrounds: a dot has no room for the badge's own light-background/
// dark-text combo, so this maps the same variants to one solid color.
export const STATUS_DOT_COLOR: Record<AppStatusSummary['variant'], string> = {
  success: 'bg-green-500 dark:bg-green-400',
  destructive: 'bg-destructive',
  muted: 'bg-muted-foreground/40',
}
