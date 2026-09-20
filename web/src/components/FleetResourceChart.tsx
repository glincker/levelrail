import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { GaugeIcon } from '@phosphor-icons/react/dist/ssr'
import { Area, AreaChart, ResponsiveContainer, Tooltip } from 'recharts'
import { appResourceUsageQueryOptions } from '../queries/appResourceUsage'
import type { AppResourceUsage } from '../types/appResourceUsage'
import {
  chartAccessibleSummary,
  formatMetricValue,
  latestReading,
  type ChartRow,
  type ChartUnit,
} from '../lib/metricChart'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const POLL_INTERVAL_MS = 30_000
const HISTORY_CAP = 30

interface FleetTotals {
  cpuTotal: number
  cpuCount: number
  memoryTotal: number
  memoryCount: number
}

function sumUsage(data: AppResourceUsage[]): FleetTotals {
  let cpuTotal = 0
  let cpuCount = 0
  let memoryTotal = 0
  let memoryCount = 0
  for (const usage of data) {
    if (usage.cpu_percent !== undefined) {
      cpuTotal += usage.cpu_percent
      cpuCount += 1
    }
    if (usage.memory_usage_bytes !== undefined) {
      memoryTotal += usage.memory_usage_bytes
      memoryCount += 1
    }
  }
  return { cpuTotal, cpuCount, memoryTotal, memoryCount }
}

// Dashboard-home trend for total CPU/memory across every app. GET
// /api/v1/apps/resource-usage (queries/appResourceUsage.ts, the same
// endpoint TopResourceConsumers reads) only ever reports each app's
// latest sample, not a time series, so this polls that one lightweight
// aggregated response (never one request per app) and accumulates a
// bounded client-side history to draw a trend from.
type HistoryPoint = { t: number; cpuTotal: number; memoryTotal: number }

export function FleetResourceChart() {
  const { data, isPending, isError, dataUpdatedAt } = useQuery({
    ...appResourceUsageQueryOptions(),
    retry: false,
    refetchInterval: POLL_INTERVAL_MS,
  })

  // "Storing information from previous renders"
  // (https://react.dev/reference/react/useState#storing-information-from-previous-renders):
  // dataUpdatedAt is react-query's own fetch timestamp, not a Date.now()
  // call of this component's own, so comparing against it and calling
  // setState conditionally here is a pure render, not an effect mirroring
  // a prop into state.
  const [tracked, setTracked] = useState<{
    lastUpdatedAt: number
    points: HistoryPoint[]
  }>({ lastUpdatedAt: 0, points: [] })
  if (data && dataUpdatedAt !== tracked.lastUpdatedAt) {
    const { cpuTotal, memoryTotal } = sumUsage(data)
    setTracked({
      lastUpdatedAt: dataUpdatedAt,
      points: [
        ...tracked.points,
        { t: dataUpdatedAt, cpuTotal, memoryTotal },
      ].slice(-HISTORY_CAP),
    })
  }
  const history = tracked.points

  if (isPending || isError || !data || data.length === 0) {
    return null
  }

  const { cpuCount, memoryCount } = sumUsage(data)
  if (cpuCount === 0 && memoryCount === 0) {
    return null
  }

  const cpuRows: ChartRow[] = history.map((p) => ({
    t: p.t,
    primary: p.cpuTotal,
  }))
  const memoryRows: ChartRow[] = history.map((p) => ({
    t: p.t,
    primary: p.memoryTotal,
  }))

  return (
    <Card size="sm">
      <CardHeader className="flex-row items-center justify-between gap-3 space-y-0">
        <CardTitle className="flex items-center gap-1.5 text-sm font-medium">
          <GaugeIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          Fleet resource usage
        </CardTitle>
        <span className="text-xs text-muted-foreground">
          Across {data.length} app{data.length === 1 ? '' : 's'}
        </span>
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <FleetSparkline
          label="Total CPU"
          unit="percent"
          color="#0ea5e9"
          rows={cpuRows}
          hasData={cpuCount > 0}
        />
        <FleetSparkline
          label="Total memory"
          unit="bytes"
          color="#a855f7"
          rows={memoryRows}
          hasData={memoryCount > 0}
        />
      </CardContent>
    </Card>
  )
}

function FleetSparkline({
  label,
  unit,
  color,
  rows,
  hasData,
}: {
  label: string
  unit: ChartUnit
  color: string
  rows: ChartRow[]
  hasData: boolean
}) {
  const current = hasData ? latestReading(rows, unit) : null

  return (
    <div>
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">{label}</p>
        {current !== null ? (
          <p className="font-mono text-sm font-medium text-foreground">
            {current}
          </p>
        ) : null}
      </div>
      {!hasData ? (
        <p className="mt-1 flex h-16 items-center text-xs text-muted-foreground">
          No data yet.
        </p>
      ) : rows.length < 2 ? (
        <p className="mt-1 flex h-16 items-center text-xs text-muted-foreground">
          Collecting data...
        </p>
      ) : (
        <div
          className="mt-1 h-16"
          role="img"
          aria-label={chartAccessibleSummary(rows, unit, label)}
        >
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart
              data={rows}
              margin={{ top: 4, right: 0, left: 0, bottom: 0 }}
            >
              <Tooltip
                labelFormatter={(t) => new Date(Number(t)).toLocaleTimeString()}
                formatter={(value) => formatMetricValue(unit, Number(value))}
              />
              <Area
                type="monotone"
                dataKey="primary"
                stroke={color}
                fill={color}
                fillOpacity={0.15}
                strokeWidth={1.5}
                dot={false}
                isAnimationActive={false}
                connectNulls
              />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}
