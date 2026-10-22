import { useState } from 'react'
import { useDatabaseMetricSeries } from '../../../queries/databaseMetrics'
import type { DatabaseResource } from '../../../types/databaseDetail'
import { latestValue, peakValue } from './sizingHelpers'

const WINDOW_MS = 3_600_000
const MINUTE_MS = 60_000
const MAX_POINTS = 120

// Window end is floored to the minute so every caller shares one cache key.
export function useResourceUsage(database: DatabaseResource) {
  const [range] = useState(() => {
    const to = new Date(Math.floor(Date.now() / MINUTE_MS) * MINUTE_MS)
    return {
      from: new Date(to.getTime() - WINDOW_MS),
      to,
      maxPoints: MAX_POINTS,
    }
  })
  const enabled = !database.suspended
  const mem = useDatabaseMetricSeries(
    database.name,
    'memory_usage_bytes',
    range,
    { enabled },
  )
  const cpu = useDatabaseMetricSeries(database.name, 'cpu_percent', range, {
    enabled,
  })
  return {
    running: enabled,
    memNow: latestValue(mem.data),
    memPeak: peakValue(mem.data),
    cpuNow: latestValue(cpu.data),
    cpuPeak: peakValue(cpu.data),
  }
}
