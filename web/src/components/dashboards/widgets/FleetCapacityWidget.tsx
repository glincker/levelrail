import { useTranslation } from 'react-i18next'
import { formatBytes } from '@/lib/format'
import { useFleetResourceUsage } from '@/queries/fleetResourceUsage'
import { Meter, Muted, WidgetSkeleton } from './parts'

interface Row {
  key: 'cpu' | 'memory' | 'disk'
  percent?: number
  detail?: string
}

/** CPU, memory and disk across nodes that report a reading. */
export function FleetCapacityWidget() {
  const { t } = useTranslation('dashboards')
  const q = useFleetResourceUsage()
  if (q.isLoading) return <WidgetSkeleton />
  if (q.error || !q.data)
    return <Muted>{t('fleet.capacity.unavailable')}</Muted>
  const f = q.data.fleet
  const cpuNodes = q.data.nodes.filter((n) => n.cpu_percent !== undefined)
  const cpu = cpuNodes.length
    ? cpuNodes.reduce((a, n) => a + (n.cpu_percent ?? 0), 0) / cpuNodes.length
    : undefined
  const rows: Row[] = [
    { key: 'cpu', percent: cpu },
    {
      key: 'memory',
      percent: f.memory_used_percent,
      detail:
        f.total_memory_bytes && f.total_memory_usage_bytes !== undefined
          ? `${formatBytes(f.total_memory_usage_bytes)} / ${formatBytes(f.total_memory_bytes)}`
          : undefined,
    },
    {
      key: 'disk',
      percent: f.disk_used_percent,
      detail:
        f.total_disk_bytes && f.total_disk_used_bytes !== undefined
          ? `${formatBytes(f.total_disk_used_bytes)} / ${formatBytes(f.total_disk_bytes)}`
          : undefined,
    },
  ]
  if (rows.every((r) => r.percent === undefined)) {
    return <Muted>{t('fleet.capacity.empty')}</Muted>
  }
  return (
    <ul className="space-y-2">
      {rows.map((r) => (
        <li key={r.key} className="space-y-1">
          <div className="flex justify-between gap-3 text-sm">
            <span>{t(`fleet.capacity.${r.key}`)}</span>
            <span className="tabular-nums text-muted-foreground">
              {r.percent === undefined
                ? t('widget.unknown')
                : `${Math.round(r.percent)}%`}
              {r.detail ? ` (${r.detail})` : ''}
            </span>
          </div>
          <Meter
            percent={r.percent ?? 0}
            label={t(`fleet.capacity.${r.key}`)}
          />
        </li>
      ))}
    </ul>
  )
}
