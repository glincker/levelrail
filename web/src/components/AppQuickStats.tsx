import { CpuIcon, GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import { useMetricSeries } from '../queries/metrics'
import { formatBytes } from '../lib/format'
import { Card, CardContent } from '@/components/ui/card'

// A small "at a glance" row above the fold, the same spirit as other
// deploy platforms' overview quick-stat cards, built from only the
// metrics this codebase actually collects (internal/telemetry's 7
// container-resource metrics). Deliberately does not include a request
// rate or error rate card: those need ingress-layer instrumentation
// that doesn't exist yet (see types/metrics.ts's own doc comment), so
// showing one here would mean fabricating a number, not reporting a
// real one.
export function AppQuickStats({ appName }: { appName: string }) {
  const now = new Date()
  const range = {
    from: new Date(now.getTime() - 5 * 60 * 1000),
    to: now,
  }

  const cpu = useMetricSeries(appName, 'cpu_percent', range)
  const memUsage = useMetricSeries(appName, 'memory_usage_bytes', range)
  const memLimit = useMetricSeries(appName, 'memory_limit_bytes', range)

  // 501 means telemetry isn't configured on this control plane at all
  // (WithMetricsStore never called): match MetricsDashboard.tsx's own
  // "hide, don't error" choice for that case, since it's a deliberate
  // operator choice, not a bug. Any other error still renders nothing
  // here rather than a broken-looking stat card; the full Metrics page
  // this row links to is where a real error surfaces properly.
  if (cpu.isError || memUsage.isError) {
    return null
  }

  const cpuValue = latestValue(cpu.data?.points)
  const memUsageValue = latestValue(memUsage.data?.points)
  const memLimitValue = latestValue(memLimit.data?.points)

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-2">
      <StatCard
        icon={CpuIcon}
        label="CPU"
        value={cpuValue === null ? '—' : `${cpuValue.toFixed(1)}%`}
        appName={appName}
      />
      <StatCard
        icon={GaugeIcon}
        label="Memory"
        value={
          memUsageValue === null
            ? '—'
            : memLimitValue
              ? `${formatBytes(memUsageValue)} / ${formatBytes(memLimitValue)}`
              : formatBytes(memUsageValue)
        }
        appName={appName}
      />
    </div>
  )
}

function latestValue(points?: { value: number }[]): number | null {
  if (!points || points.length === 0) return null
  const last = points[points.length - 1]
  return last ? last.value : null
}

function StatCard({
  icon: StatIcon,
  label,
  value,
  appName,
}: {
  icon: typeof CpuIcon
  label: string
  value: string
  appName: string
}) {
  return (
    <Link to="/apps/$name/metrics" params={{ name: appName }}>
      <Card className="transition-colors hover:border-primary/40">
        <CardContent className="flex items-center gap-3 py-4">
          <StatIcon className="size-5 shrink-0 text-muted-foreground" />
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground uppercase tracking-wide">
              {label}
            </p>
            <p className="text-lg font-semibold text-foreground">{value}</p>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
