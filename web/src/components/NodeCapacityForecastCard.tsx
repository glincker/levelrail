import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useNodeCapacityForecast } from '../queries/nodes'
import type { CapacityForecastMetric } from '../types/nodeDetail'
import { formatBytes } from '../lib/format'

// GET /api/v1/nodes/{id}/capacity-forecast (internal/api/
// node_capacity_forecast.go): a rough "days until full" projection for
// this node's disk and memory, fit from a plain linear trend over
// recent history (internal/forecast), never from an external model.
//
// Deliberately renders nothing, not a reassuring "all good" state, when
// there's no real growing trend yet: an absent `disk`/`memory` field
// means the fitted slope is flat or improving, or there isn't enough
// history, and a "capacity looks fine forever" card is the kind of
// claim that ages badly the day usage actually turns upward. Loading
// and error states render nothing for the same reason this is a
// heads-up widget, not load-bearing status: a node detail page with a
// missing patch-status/mesh card already treats telemetry-not-configured
// as "nothing to show" rather than an error banner.
function daysLabel(days: number): string {
  if (days < 1) {
    return 'less than a day'
  }
  const rounded = Math.round(days)
  return `about ${rounded} day${rounded === 1 ? '' : 's'}`
}

function variantFor(days: number): 'destructive' | 'warning' {
  return days <= 7 ? 'destructive' : 'warning'
}

function ForecastRow({
  label,
  metric,
}: {
  label: string
  metric: CapacityForecastMetric
}) {
  return (
    <div className="flex items-start gap-2">
      <Badge variant={variantFor(metric.days_until_full)} className="mt-0.5">
        <WarningIcon className="size-3" />
        {daysLabel(metric.days_until_full)}
      </Badge>
      <p className="text-sm text-foreground">
        <span className="font-medium">{label}</span> will be full in{' '}
        {daysLabel(metric.days_until_full)} at the current growth rate (
        {formatBytes(metric.current_used_bytes)} of{' '}
        {formatBytes(metric.total_bytes)} used, growing{' '}
        {formatBytes(metric.slope_bytes_per_day)}/day).
      </p>
    </div>
  )
}

export function NodeCapacityForecastCard({
  nodeId,
}: Readonly<{ nodeId: string }>) {
  const { data, isPending, isError } = useNodeCapacityForecast(nodeId)

  if (isPending || isError || !data || (!data.disk && !data.memory)) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Capacity forecast</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {data.disk ? <ForecastRow label="Disk" metric={data.disk} /> : null}
        {data.memory ? (
          <ForecastRow label="Memory" metric={data.memory} />
        ) : null}
        <p className="text-xs text-muted-foreground">{data.note}</p>
      </CardContent>
    </Card>
  )
}
