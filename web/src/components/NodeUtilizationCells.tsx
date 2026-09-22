import type { NodeResourceUsage } from '../types/fleetResourceUsage'
import { formatBytes } from '../lib/format'
import { Progress } from '@/components/ui/progress'

// Three small cells the node list's grid renders side by side (CPU,
// Memory, Disk), all reading one shared NodeResourceUsage row so
// routes/nodes/index.tsx only has to look the row up once per node. Kept
// as one file rather than three since none is useful without the others
// and they share formatPercent/UsageBar below.

function formatPercent(value?: number): string {
  if (value === undefined) return '-'
  return `${value.toFixed(1)}%`
}

// UsageBar is deliberately not color-thresholded (amber/destructive):
// this component has no access to this project's real alert thresholds
// (internal/alerting, env-var configurable per the "no hardcoded
// thresholds" rule), and inventing a second, UI-only threshold here
// would drift from whatever the operator actually configured. The
// node detail page's alert_status badge is the place that reflects real
// threshold state; this bar is a plain visual proportion only.
function UsageBar({ value }: { value: number }) {
  return <Progress value={Math.min(value, 100)} />
}

// CPUPercent is the sum of every placed container's own cpu_percent, not
// a read against total core count (see NodeResourceUsage's own doc
// comment): no bar here, since there is no known 100%-of-capacity
// denominator to draw one against, only the raw number.
export function NodeCPUCell({ usage }: { usage?: NodeResourceUsage }) {
  return (
    <span className="truncate text-xs text-muted-foreground tabular-nums">
      {formatPercent(usage?.cpu_percent)}
    </span>
  )
}

export function NodeMemoryCell({ usage }: { usage?: NodeResourceUsage }) {
  if (!usage || usage.memory_usage_bytes === undefined) {
    return <span className="text-xs text-muted-foreground">-</span>
  }
  if (usage.memory_total_bytes === undefined || usage.memory_total_bytes <= 0) {
    return (
      <span className="truncate text-xs text-muted-foreground tabular-nums">
        {formatBytes(usage.memory_usage_bytes)}
      </span>
    )
  }
  const pct = (usage.memory_usage_bytes / usage.memory_total_bytes) * 100
  return (
    <div className="min-w-0 space-y-1">
      <span className="block truncate text-xs text-muted-foreground tabular-nums">
        {formatBytes(usage.memory_usage_bytes)} /{' '}
        {formatBytes(usage.memory_total_bytes)}
      </span>
      <UsageBar value={pct} />
    </div>
  )
}

export function NodeDiskCell({ usage }: { usage?: NodeResourceUsage }) {
  if (
    !usage ||
    usage.disk_used_bytes === undefined ||
    usage.disk_total_bytes === undefined ||
    usage.disk_total_bytes <= 0
  ) {
    return <span className="text-xs text-muted-foreground">-</span>
  }
  const pct = (usage.disk_used_bytes / usage.disk_total_bytes) * 100
  return (
    <div className="min-w-0 space-y-1">
      <span className="block truncate text-xs text-muted-foreground tabular-nums">
        {formatBytes(usage.disk_used_bytes)} /{' '}
        {formatBytes(usage.disk_total_bytes)}
      </span>
      <UsageBar value={pct} />
    </div>
  )
}
