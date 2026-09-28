import { useMetricSeries } from '../../queries/metrics'
import type { AppDetail } from '../../types/appDetail'
import { TRAFFIC_REFETCH_MS } from './config'
import { useSlidingRange } from './useSlidingRange'

const RANGE_MINUTES = 15
const NANO = 1e9

export function usageRatio(value: number | null, limit: number): number {
  if (value === null || limit <= 0) return 0
  return Math.min(value / limit, 1)
}

function last(points?: { value: number }[]): number | null {
  return points?.at(-1)?.value ?? null
}

export interface ResourceReading {
  isPending: boolean
  isError: boolean
  cpuNow: number | null
  memNow: number | null
  memLimit: number
  cores: number
  cpuSeries: number[]
  memSeries: number[]
}

export function useResourceReading(app: AppDetail): ResourceReading {
  const range = useSlidingRange(RANGE_MINUTES, TRAFFIC_REFETCH_MS)
  const cpu = useMetricSeries(app.name, 'cpu_percent', range)
  const mem = useMetricSeries(app.name, 'memory_usage_bytes', range)
  const limit = useMetricSeries(app.name, 'memory_limit_bytes', range)
  return {
    isPending: cpu.isPending || mem.isPending,
    isError: cpu.isError || mem.isError,
    cpuNow: last(cpu.data?.points),
    memNow: last(mem.data?.points),
    memLimit: app.resources?.memory_bytes ?? last(limit.data?.points) ?? 0,
    cores: (app.resources?.nano_cpus ?? 0) / NANO,
    cpuSeries: (cpu.data?.points ?? []).map((p) => p.value),
    memSeries: (mem.data?.points ?? []).map((p) => p.value),
  }
}
