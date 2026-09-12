import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ChartBarIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppListEntry } from '../types/appDetail'
import type { AppResourceUsage } from '../types/appResourceUsage'
import { useAppResourceUsage } from '../queries/appResourceUsage'
import { formatBytes } from '../lib/format'
import { StopStartAppButton } from './StopStartAppButton'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

const ROW_CAP = 5

type RankMetric = 'cpu_percent' | 'memory_usage_bytes' | 'network_rx_bytes'

const RANK_OPTIONS: { key: RankMetric; label: string }[] = [
  { key: 'cpu_percent', label: 'CPU' },
  { key: 'memory_usage_bytes', label: 'Memory' },
  { key: 'network_rx_bytes', label: 'Network' },
]

function valueOf(usage: AppResourceUsage, metric: RankMetric): number {
  return usage[metric] ?? 0
}

function formatValue(usage: AppResourceUsage, metric: RankMetric): string {
  switch (metric) {
    case 'cpu_percent':
      return usage.cpu_percent === undefined ? '-' : `${usage.cpu_percent.toFixed(1)}%`
    case 'memory_usage_bytes':
      if (usage.memory_usage_bytes === undefined) return '-'
      if (usage.memory_limit_bytes) {
        return `${formatBytes(usage.memory_usage_bytes)} / ${formatBytes(usage.memory_limit_bytes)}`
      }
      return formatBytes(usage.memory_usage_bytes)
    case 'network_rx_bytes':
      return usage.network_rx_bytes === undefined
        ? '-'
        : `${formatBytes(usage.network_rx_bytes)} in / ${formatBytes(usage.network_tx_bytes)} out`
  }
}

// Dashboard panel ranking every app by its latest CPU/memory/network
// reading (GET /api/v1/apps/resource-usage), so "which app is eating my
// CPU" or "which app is getting hammered with traffic" is a glance, not
// an investigation across each app's own metrics tab. Renders nothing
// when telemetry isn't configured (isError) or no app has ever reported
// a sample (data.length === 0): an empty or broken panel on every
// dashboard load would be worse than not showing it at all.
export function TopResourceConsumers({ apps }: { apps: AppListEntry[] }) {
  const [rankBy, setRankBy] = useState<RankMetric>('cpu_percent')
  const { data, isPending, isError } = useAppResourceUsage()

  if (isPending || isError || !data || data.length === 0) {
    return null
  }

  const appsByName = new Map(apps.map((a) => [a.name, a]))
  const ranked = [...data]
    .filter((u) => valueOf(u, rankBy) > 0 || appsByName.has(u.name))
    .sort((a, b) => valueOf(b, rankBy) - valueOf(a, rankBy))
    .slice(0, ROW_CAP)

  if (ranked.length === 0) {
    return null
  }

  return (
    <Card size="sm">
      <CardHeader className="flex-row items-center justify-between gap-3 space-y-0">
        <CardTitle className="flex items-center gap-1.5 text-sm font-medium">
          <ChartBarIcon className="size-4 text-muted-foreground" aria-hidden="true" />
          Top resource consumers
        </CardTitle>
        <div className="flex gap-1">
          {RANK_OPTIONS.map((opt) => (
            <Button
              key={opt.key}
              size="sm"
              variant={rankBy === opt.key ? 'secondary' : 'ghost'}
              onClick={() => setRankBy(opt.key)}
              className="h-6 px-2 text-xs"
            >
              {opt.label}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="divide-y divide-border p-0">
        {ranked.map((usage) => {
          const app = appsByName.get(usage.name)
          return (
            <div
              key={usage.name}
              className="flex items-center gap-3 px-4 py-2.5 text-sm"
            >
              <Link
                to="/apps/$name"
                params={{ name: usage.name }}
                className="min-w-0 flex-1 truncate font-medium text-foreground hover:underline"
              >
                {usage.name}
              </Link>
              <span className="shrink-0 font-mono text-xs text-muted-foreground">
                {formatValue(usage, rankBy)}
              </span>
              {app ? (
                <StopStartAppButton name={app.name} suspended={app.suspended} />
              ) : null}
            </div>
          )
        })}
      </CardContent>
    </Card>
  )
}
